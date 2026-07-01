"use strict";

(function () {
  const { $, api, toast, escapeHtml } = window.panel;
  let loaded = false;

  document.addEventListener("panel:view", (ev) => {
    if (ev.detail.view === "settings" && !loaded) {
      loadSettings();
    }
  });

  const refreshBtn = document.getElementById("settings-refresh");
  if (refreshBtn) refreshBtn.addEventListener("click", loadSettings);

  async function loadSettings() {
    const host = $("#settings-list");
    host.innerHTML = "<p class='muted'>loading…</p>";
    try {
      const data = await api("GET", "/v1/settings");
      render(data.settings || []);
      loaded = true;
    } catch (e) {
      host.innerHTML = `<p class='muted'>error: ${escapeHtml(e.message)}</p>`;
    }
  }

  function render(settings) {
    const host = $("#settings-list");
    host.innerHTML = "";
    const groups = {};
    for (const s of settings) {
      (groups[s.group] ||= []).push(s);
    }
    for (const group of Object.keys(groups)) {
      const section = document.createElement("section");
      const h = document.createElement("h3");
      h.textContent = group;
      section.append(h);
      for (const s of groups[group]) {
        section.append(row(s));
      }
      host.append(section);
    }
  }

  function row(s) {
    const wrap = document.createElement("div");
    wrap.className = "setting-row";
    const readOnly = s.apply === "bootstrap";

    const label = document.createElement("label");
    label.textContent = s.label || s.key;
    if (s.apply === "restart") {
      const badge = document.createElement("span");
      badge.className = "badge";
      badge.textContent = "restart required";
      label.append(" ", badge);
    }

    const input = document.createElement("input");
    input.dataset.key = s.key;
    if (s.kind === "bool") {
      input.type = "checkbox";
      input.checked = String(s.value).toLowerCase() === "true";
    } else if (s.secret) {
      input.type = "password";
      input.placeholder = s.is_set ? "•••••• (set)" : "(unset)";
    } else {
      input.type = s.kind === "int" ? "number" : "text";
      input.value = s.value || "";
    }
    input.disabled = readOnly;

    const help = document.createElement("p");
    help.className = "muted help";
    help.textContent = `${s.key} · source: ${s.source}${s.help ? " · " + s.help : ""}`;

    wrap.append(label, input);
    if (!readOnly) {
      const save = document.createElement("button");
      save.textContent = "Save";
      save.addEventListener("click", () => saveSetting(s, input));
      const reset = document.createElement("button");
      reset.textContent = "Reset";
      reset.addEventListener("click", () => resetSetting(s.key));
      wrap.append(save, reset);
    }
    wrap.append(help);
    return wrap;
  }

  async function saveSetting(s, input) {
    let value;
    if (s.kind === "bool") {
      value = input.checked ? "true" : "false";
    } else {
      value = input.value;
      if (s.secret && value === "") {
        toast("enter a value to set this secret");
        return;
      }
    }
    try {
      await api("PUT", "/v1/settings", {
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ key: s.key, value }),
      });
      toast(`${s.key} saved`);
      loaded = false;
      loadSettings();
    } catch (e) {
      toast(e.message);
    }
  }

  async function resetSetting(key) {
    if (!confirm(`Reset ${key} to its environment/default value?`)) return;
    try {
      await api("DELETE", `/v1/settings/${encodeURIComponent(key)}`);
      toast(`${key} reset`);
      loaded = false;
      loadSettings();
    } catch (e) {
      toast(e.message);
    }
  }
})();
