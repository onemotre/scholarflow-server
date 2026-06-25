# Daily arXiv Harvest — Design

Date: 2026-06-25
Status: Approved (brainstorming)
Module: `scholarflow-server`

## Goal

Automatically ingest newly-posted arXiv papers every day for a configured set of
arXiv categories, feeding them into the existing processing pipeline so they
appear via the normal API alongside manually-uploaded PDFs.

## Decisions (from brainstorming)

- **Trigger**: daily scheduled job (not on-demand ID submission). No new HTTP endpoint.
- **Criteria**: arXiv categories (e.g. `cs.CL`, `cs.AI`).
- **Config**: all via env vars, consistent with `internal/config`. Default off.
- **LLM read**: harvested papers follow the *same* pipeline as uploads — if a
  reader is configured they get a paper card. (Reader is off by default.)
- **Extensibility**: lightly pluggable. A generic `Source` interface (arXiv is
  the only impl now) so future sources (bioRxiv, Semantic Scholar, RSS) are
  drop-in without building any second source today. No registry / dynamic
  registration — YAGNI.

## Approach

A new daily worker task `arxiv:harvest` queries the arXiv Query API for the most
recently submitted papers per subscribed category, downloads each PDF into MinIO,
and enqueues the existing `paper:process` task. Harvested papers are
indistinguishable downstream except `source_type = 'arxiv'` and a populated
`source_id` (the arXiv id). Maximizes reuse; the change is additive.

Why the Query API (not RSS or OAI-PMH): flexible category + sort-by-submittedDate
filtering, simple Atom XML, idempotent via dedup. OAI-PMH would close the
"more than MAX_RESULTS/day" gap but is out of scope.

## Components

### `internal/sources/` (new package — the pluggable seam)

Source-agnostic types so the harvest machinery never imports arXiv directly.

```go
type Entry struct {
    SourceID, Title, Abstract, DOI, PrimaryCategory string // SourceID = normalized arxiv id, etc.
    Authors   []string
    Published  time.Time
    PDFURL     string
}

type Source interface {
    Name() string // e.g. "arxiv" -> becomes papers.source_type
    FetchRecent(ctx context.Context, category string, maxResults int) ([]Entry, error)
}
```

`HarvestPipeline` depends only on `sources.Source` + `sources.Entry`. Adding a
second source later = implement the interface and pass it in; no pipeline change.

### `internal/arxiv/` (new package — first `Source` impl)

Thin client over `http://export.arxiv.org/api/query` implementing
`sources.Source` (`Name() == "arxiv"`), behind the interface for unit-testability.

- `client.go` (real impl): builds
  `search_query=cat:<cat>&sortBy=submittedDate&sortOrder=descending&max_results=N`,
  parses the Atom feed (`encoding/xml`), sets a polite `User-Agent`, and sleeps
  `ARXIV_REQUEST_DELAY_SECONDS` (default 3) between requests per arXiv guidance.
- ID normalization: strip version suffix (`2301.00001v2` -> `2301.00001`) so dedup
  is stable across paper revisions.
- `Base URL` is configurable (`ARXIV_API_BASE_URL`) so tests can point at a fixture server.

### `internal/jobs/` (harvest orchestration)

Mirrors the existing pipeline/processor split.

- `tasks.go`: add `TypeHarvestArxiv = "arxiv:harvest"` + `NewHarvestArxivTask()`.
- `harvest_pipeline.go`: `HarvestPipeline` — testable service behind interfaces
  (`[]sources.Source`, `storage.Store`, a harvest repo, the existing `Enqueuer`).
  Per source -> per category -> per entry (`source_type = source.Name()`):
  1. **Dedup** via `GetPaperBySourceID(arxivID)`; skip if present.
  2. **Download** PDF from `entry.PDFURL` (HTTP GET into memory; papers fit under
     the 50MB upload cap).
  3. **Store** in MinIO via `storage.Store.Put`.
  4. **Create** `papers`/`paper_assets`/`paper_processing_jobs` rows with
     `source_type='arxiv'`, `source_id=arxivID`, arXiv metadata pre-filled.
  5. **Enqueue** `paper:process` (existing `Enqueuer`), set job task id.
  - Per-entry failures are logged and skipped (best-effort). The task returns nil
    so it does not noisily retry; dedup makes re-runs idempotent.
- `harvest_processor.go`: thin asynq adapter delegating to `HarvestPipeline`.

### `internal/papers/` (ingestion refactor)

Today `Service.UploadPDF` and `SQLRepository.CreatePaperUpload` hard-code
`source_type='local_pdf'`. Generalize:

- Introduce `SourceInfo{ SourceType, SourceID, Filename, Title, Abstract, DOI, Year }`.
- `Service.IngestPDF(ctx, SourceInfo, body, size, contentType) (UploadResult, error)`
  carries the shared store->create->enqueue logic.
- `UploadPDF` becomes a thin wrapper: `IngestPDF` with `SourceType="local_pdf"`,
  empty `SourceID`, no metadata. Preserves existing behavior and tests.
- `Repository.CreatePaperUpload` generalized to accept source params; add
  `GetPaperBySourceID(ctx, sourceID) (exists bool, err error)` for dedup
  (existence check only; the paper id is not needed by the harvest path).

### Schema — migration `00003_paper_source_id.sql`

- `ALTER TABLE papers ADD COLUMN source_id TEXT;`
- Partial unique index:
  `CREATE UNIQUE INDEX papers_arxiv_source_id_key ON papers(source_id) WHERE source_type='arxiv';`
  (race safety net).
- Update `queries/papers.sql`: `CreatePaper` gains `source_id`, `title`,
  `abstract`, `doi`, `publication_year`; new `GetPaperBySourceID`.
- Regenerate `internal/db` with `sqlc generate` (do not hand-edit `*.sql.go`).
- Note: GROBID parse still updates title/abstract/doi/year afterwards; arXiv
  pre-fill provides immediate metadata and a fallback.

### Config — `internal/config/config.go` + `.env.example` (all default-off)

| Env var | Default | Meaning |
|---|---|---|
| `ARXIV_HARVEST_ENABLED` | `false` | master switch |
| `ARXIV_HARVEST_CATEGORIES` | `""` | csv, e.g. `cs.CL,cs.AI` |
| `ARXIV_HARVEST_CRON` | `@daily` | asynq cron spec |
| `ARXIV_HARVEST_MAX_RESULTS` | `50` | per-category cap |
| `ARXIV_API_BASE_URL` | `http://export.arxiv.org/api/query` | for tests |
| `ARXIV_REQUEST_DELAY_SECONDS` | `3` | politeness delay |

### Wiring — `cmd/worker/main.go`

- Register `harvest_processor` on the asynq mux (always; cheap no-op if never enqueued).
- When `ARXIV_HARVEST_ENABLED`, register a `@cron` entry on the existing
  `asynq.Scheduler` (alongside the cleanup cron) using `ARXIV_HARVEST_CRON`.
- `cmd/server` unchanged.

## Testing (no Docker; via interfaces)

- `internal/arxiv`: Atom XML fixture -> parsed `[]Entry`; ID-normalization table tests;
  query-URL construction.
- `internal/jobs`: `HarvestPipeline` with fake `sources.Source`/`storage.Store`/repo/
  `Enqueuer` — asserts dedup-skip, download->create->enqueue happy path, and
  per-entry error isolation (one bad entry doesn't abort the batch).
- `internal/papers`: existing upload tests stay green; add an arxiv-source
  `IngestPDF` test.

## Error handling

- Category fetch failure: log, continue to next category.
- Per-entry (download/store/create/enqueue) failure: log, skip entry, continue.
- Harvest task returns nil after best-effort to avoid whole-batch retries;
  idempotent dedup covers the next scheduled run.

## Caveats / out of scope

- "New every day" = top `MAX_RESULTS` by submittedDate per category, deduped.
  Robust to missed days and re-runs, but misses the tail if a category posts more
  than `MAX_RESULTS` in a day. OAI-PMH incremental harvest would close this gap —
  out of scope.
- Same-as-uploads read means category harvest can incur significant LLM cost when
  a reader is configured. Safe by default (reader off).
- No runtime subscription management (no DB table / CRUD API); categories change
  via env + redeploy.

## CHANGELOG

This is a version-level feature; add a `CHANGELOG.md` entry (top-level changelog)
on implementation.
