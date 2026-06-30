"use strict";

const $ = (sel) => document.querySelector(sel);
const tokenKey = "scholarflow_write_token";

function getToken() { return localStorage.getItem(tokenKey) || ""; }
function setToken(t) { localStorage.setItem(tokenKey, t); reflectToken(); }
function clearToken() { localStorage.removeItem(tokenKey); reflectToken(); }
function reflectToken() {
  const t = getToken();
  $("#token").value = t;
  $("#token-state").textContent = t ? "token set" : "no token (writes work only if auth is disabled)";
}

function authHeaders() {
  const t = getToken();
  return t ? { Authorization: "Bearer " + t } : {};
}

function toast(msg) {
  const el = $("#toast");
  el.textContent = msg;
  el.hidden = false;
  clearTimeout(toast._t);
  toast._t = setTimeout(() => { el.hidden = true; }, 3000);
}
async function api(method, url, opts = {}) {
  const headers = method === "GET" ? {} : authHeaders();
  const resp = await fetch(url, { method, headers: { ...headers, ...(opts.headers || {}) }, body: opts.body });
  if (!resp.ok) {
    const text = await resp.text();
    throw new Error(`${resp.status}: ${text.trim() || resp.statusText}`);
  }
  const ct = resp.headers.get("Content-Type") || "";
  return ct.includes("application/json") ? resp.json() : null;
}

function fmtDate(s) { return s ? new Date(s).toLocaleString() : ""; }

function sourceLabel(p) {
  const parts = [p.source_type];
  if (p.primary_category) parts.push(p.primary_category);
  return parts.filter(Boolean).join(" · ");
}

async function loadPapers() {
  const tbody = $("#papers tbody");
  tbody.innerHTML = "<tr><td colspan='6' class='muted'>loading…</td></tr>";
  try {
    const papers = await api("GET", "/v1/papers");
    if (!papers.length) { tbody.innerHTML = "<tr><td colspan='6' class='muted'>no papers</td></tr>"; return; }
    tbody.innerHTML = "";
    for (const p of papers) {
      const tr = document.createElement("tr");
      tr.innerHTML = `
        <td>${escapeHtml(p.title || "(untitled)")}</td>
        <td>${escapeHtml(sourceLabel(p))}</td>
        <td class="status">${escapeHtml(p.status)}</td>
        <td>${escapeHtml(p.publication_year || "")}</td>
        <td>${fmtDate(p.created_at)}</td>
        <td class="actions"></td>`;
      const cell = tr.querySelector(".actions");
      cell.append(
        actionBtn("View", () => viewPaper(p.paper_id)),
        actionBtn("Re-process", () => runAction(p.paper_id, "reprocess")),
        actionBtn("Regen card", () => runAction(p.paper_id, "reread")),
        actionBtn("Delete", () => deletePaper(p.paper_id), true),
      );
      tbody.append(tr);
    }
  } catch (e) {
    tbody.innerHTML = `<tr><td colspan='6' class='muted'>error: ${escapeHtml(e.message)}</td></tr>`;
  }
}

function actionBtn(label, fn, danger = false) {
  const b = document.createElement("button");
  b.textContent = label;
  if (danger) b.className = "danger";
  b.addEventListener("click", fn);
  return b;
}

function escapeHtml(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
}

// Exposed so Plan 2's settings.js can reuse the helpers without redefining them.
window.panel = { $, authHeaders, toast, api, escapeHtml };

async function runAction(id, kind) {
  try {
    const job = await api("POST", `/v1/papers/${id}/${kind}`);
    toast(`${kind}: job ${job.status}`);
    pollJob(job.job_id);
  } catch (e) { toast(e.message); }
}

async function deletePaper(id) {
  if (!confirm("Permanently delete this paper, its data, and its files?")) return;
  try {
    await api("DELETE", `/v1/papers/${id}`);
    toast("deleted");
    loadPapers();
  } catch (e) { toast(e.message); }
}

async function pollJob(jobID) {
  const terminal = new Set(["completed", "failed", "parsed"]);
  for (let i = 0; i < 30; i++) {
    await new Promise((r) => setTimeout(r, 2000));
    try {
      const job = await api("GET", `/v1/jobs/${jobID}`);
      if (terminal.has(job.status)) { toast(`job ${job.status}`); loadPapers(); return; }
    } catch { return; }
  }
  loadPapers();
}

async function viewPaper(id) {
  const dlg = $("#detail");
  dlg.innerHTML = "<p class='muted'>loading…</p>";
  dlg.showModal();
  try {
    const p = await api("GET", `/v1/papers/${id}`);
    dlg.innerHTML = `
      <h2>${escapeHtml(p.title || "(untitled)")}</h2>
      <p class="muted">status: ${escapeHtml(p.status)} · year: ${escapeHtml(p.publication_year || "?")} · file: ${escapeHtml(p.uploaded_filename)}</p>
      <p>${p.authors.length} authors · ${p.sections.length} sections · ${p.references.length} references · ${p.figures.length} figures · card: ${p.card ? "yes" : "no"}</p>
      <pre>${escapeHtml(p.abstract || "(no abstract)")}</pre>
      <form method="dialog"><button>Close</button></form>`;
  } catch (e) {
    dlg.innerHTML = `<p>error: ${escapeHtml(e.message)}</p><form method="dialog"><button>Close</button></form>`;
  }
}

function switchView(view) {
  for (const btn of document.querySelectorAll("#tabs .tab")) {
    btn.classList.toggle("active", btn.dataset.view === view);
  }
  $("#papers-view").hidden = view !== "papers";
  $("#settings-view").hidden = view !== "settings";
  // Plan 2: settings.js listens for this event to lazy-load settings on first show.
  document.dispatchEvent(new CustomEvent("panel:view", { detail: { view } }));
}

for (const btn of document.querySelectorAll("#tabs .tab")) {
  btn.addEventListener("click", () => switchView(btn.dataset.view));
}

$("#save-token").addEventListener("click", () => { setToken($("#token").value.trim()); toast("token saved"); });
$("#clear-token").addEventListener("click", () => { clearToken(); toast("token cleared"); });
$("#refresh").addEventListener("click", loadPapers);

$("#upload-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const file = $("#pdf").files[0];
  if (!file) return;
  const fd = new FormData();
  fd.append("file", file);
  try {
    const res = await api("POST", "/v1/uploads/papers", { body: fd });
    toast("uploaded");
    if (res && res.job_id) pollJob(res.job_id);
    $("#upload-form").reset();
  } catch (e) { toast(e.message); }
});

$("#harvest-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const raw = $("#categories").value.trim();
  const categories = raw ? raw.split(",").map((s) => s.trim()).filter(Boolean) : [];
  try {
    await api("POST", "/v1/harvest/arxiv", {
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ categories }),
    });
    toast("harvest triggered");
  } catch (e) { toast(e.message); }
});

reflectToken();
loadPapers();
