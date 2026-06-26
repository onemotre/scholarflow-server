# ScholarFlow Server API

## Endpoints

- `GET /healthz`
- `POST /v1/uploads/papers`
- `GET /v1/jobs/{id}`
- `POST /v1/jobs/{id}/retry`
- `GET /v1/papers/{id}`
- `GET /v1/papers/{id}/figures/{figureId}/image`
- `POST /v1/harvest/arxiv`

## Authentication

The three write endpoints — `POST /v1/uploads/papers`, `POST /v1/jobs/{id}/retry`,
and `POST /v1/harvest/arxiv` — require a bearer token when `WRITE_API_TOKEN` is
configured on the server. Send it as:

    Authorization: Bearer <WRITE_API_TOKEN>

A missing or incorrect token returns `401 unauthorized`. When `WRITE_API_TOKEN`
is blank (default), auth is disabled and these endpoints are open. Read endpoints
and `GET /healthz` are always public.

Example:

    curl -X POST http://localhost:8080/v1/uploads/papers \
      -H "Authorization: Bearer $WRITE_API_TOKEN" \
      -F file=@paper.pdf

## Local Verification

Start dependencies and services:

```bash
docker compose up
```

Check health:

```bash
curl http://localhost:8080/healthz
```

Expected:

```text
ok
```

## Read Endpoints

### `GET /v1/jobs/{id}`

Returns processing-job status. `404` if the job does not exist, `400` for a malformed UUID.

```json
{
  "job_id": "857a57d9-...",
  "paper_id": "bca2c01d-...",
  "status": "parsed",
  "attempt_count": 0,
  "error_message": null,
  "created_at": "2026-06-17T22:18:22+08:00",
  "updated_at": "2026-06-17T22:18:40+08:00",
  "completed_at": null
}
```

### `GET /v1/papers/{id}`

Returns parsed paper metadata plus authors, sections, and references. `404` if the paper does not exist, `400` for a malformed UUID.

```json
{
  "paper_id": "bca2c01d-...",
  "source_type": "local_pdf",
  "status": "parsed",
  "title": "…",
  "abstract": "…",
  "doi": null,
  "publication_year": null,
  "uploaded_filename": "paper.pdf",
  "authors": [{"order": 1, "display_name": "…", "orcid": null}],
  "sections": [{"order": 1, "heading": "Introduction", "text": "…"}],
  "references": []
}
```

`doi`, `publication_year`, and `references` are populated only insofar as the parser extracts them from the PDF; the current GROBID adapter extracts title/abstract/authors/sections, so these may be empty for preprints.

The response also includes a `figures` array (`label`, `kind` = `figure`|`table`, `caption`, `order`, `page`) extracted from the document. `doi`, `publication_year`, and `references` are populated when GROBID finds them in the PDF (preprints often have none).

### GET /v1/papers/{id}/figures/{figureId}/image

Streams the extracted PNG for a figure. `figureId` is the `id` from a figure in
`GET /v1/papers/{id}`. Responses:

- `200 OK` with `Content-Type: image/png` — the cropped figure image.
- `404 Not Found` — the figure does not exist or has no extracted image.
- `400 Bad Request` — malformed `id` or `figureId`.

Each figure object in `GET /v1/papers/{id}` now includes `"id"` (UUID) and
`"has_image"` (bool) so clients know which figures have an image to fetch.

## Worker Parse Pipeline

The `paper:process` worker task currently runs the parse-only pipeline:

1. Marks the processing job `processing`.
2. Loads the uploaded PDF asset from object storage.
3. Calls the configured parser. The first parser is GROBID.
4. Stores raw GROBID TEI XML as a `grobid_tei_xml` asset.
5. Persists parsed metadata, authors, sections, and references.
6. Marks the job `parsed`.

If parsing or persistence fails, the job is marked `failed` and the worker returns the error to asynq for retry handling.

## Reader Pipeline

When `OPENAI_BASE_URL` and `OPENAI_API_KEY` are set, a successful parse enqueues a `paper:read` task:

1. Marks the job `reading`.
2. Loads the parsed title, abstract, and sections.
3. Calls the OpenAI-compatible Chat Completions API for a JSON paper card (no `limitations` field; evidence cites sections by their order label).
4. Persists the card (`paper_cards`) and evidence (`paper_evidence`), mapping section labels to `paper_sections.id`.
5. Marks the job `completed`.

If the reader is not configured, the job stops at `parsed`. The latest card is returned in the `card` field of `GET /v1/papers/{id}`.

The card (schema `2.0`) contains the scalar fields (`background`, `problem`, `method`, `implementation`), list fields (`benchmarks`, `baselines`, `results`, `code_links`, `data_links`), a `figures` array, and an `evidence` array:

- `evidence[]`: `{claim_key, claim_index, evidence_type, section_id, page, snippet, confidence}`. `claim_index` is the 0-based item index when the evidence supports a specific bullet of a list field (else `null`). `page` is clamped server-side to the cited section's page range.
- `figures[]`: `{label, claim_key, claim_index, page}` — a placement of a figure/table at a claim anchor. `page` is filled server-side from the GROBID figure record, not the model.

Pages are PDF physical-page indices (page-level granularity); they can differ from a printed page label on offset-paginated reprints, and depend on GROBID emitting `@coords`.

### POST /v1/jobs/{id}/retry

Retries a failed processing job. Returns `409` unless the job status is
`failed`. The stage is inferred from paper state: if the paper has parsed
sections the read stage is re-run, otherwise the parse stage is re-run. The job
row is reset (`status=queued`, `attempt_count=0`, `error_message=null`) and the
appropriate task re-enqueued.

- `202 Accepted` + the reset job JSON on success
- `404 Not Found` for an unknown job id
- `409 Conflict` when the job is not in a retryable (`failed`) state
- `400 Bad Request` for a malformed job id

### POST /v1/harvest/arxiv

Manually triggers one arXiv harvest run. The request body is optional:

```json
{ "categories": ["cs.CL", "cs.CV"] }
```

- With a non-empty `categories` array, those categories override the worker's
  configured `ARXIV_HARVEST_CATEGORIES` for this run only.
- With an empty or absent body, the configured categories are used.

The endpoint enqueues an `arxiv:harvest` task and returns immediately; the worker
performs the fetch asynchronously (results appear via `GET /v1/papers`).

- `202 Accepted` + `{ "task_id": "<asynq task id>" }` on success
- `400 Bad Request` for a malformed JSON body
- `500 Internal Server Error` if enqueueing fails

Note: the worker only processes the task when its harvest processor is registered,
i.e. when `ARXIV_HARVEST_ENABLED=true` **or** `ARXIV_HARVEST_CATEGORIES` is set.

### Reader Configuration

Three optional env vars tune reader behavior (all have defaults; the reader is still disabled until `OPENAI_BASE_URL`/`OPENAI_API_KEY` are set):

- `OPENAI_API_STYLE` (default `chat`): `chat` uses `/chat/completions`; `responses` uses the OpenAI Responses API (`/responses`). Switch to `responses` only if your provider supports it.
- `OPENAI_RESPONSE_FORMAT` (default `json_schema`): enforce the full paper-card schema via Structured Outputs. Set to `json_object` if your provider rejects JSON-schema requests.
- `OPENAI_SYSTEM_PROMPT_PATH` (default empty): path to a system-prompt file that overrides the built-in default at runtime; falls back to the embedded default if unset or unreadable. **The path is resolved inside the process**, so under docker-compose it must be a container path, not a host path. The built-in default lives in `internal/reader/prompts/system.md`, which is `go:embed`-baked into the binary and is **not** present in the runtime image — `internal/reader/prompts/` is mounted read-only to `/app/prompts` by compose, so drop a custom prompt there on the host and set e.g. `OPENAI_SYSTEM_PROMPT_PATH=/app/prompts/your.md`. The prompt is loaded once at worker startup.
- `READ_MAX_RETRY` (default `3`): asynq max retries for the read task. While retries remain the job stays `reading`; it becomes `failed` only after the final attempt. Every failed attempt is logged to worker stdout.
- `JOB_FAILED_RETENTION_DAYS` (default `7`): failed job rows older than this are deleted by the daily cleanup.
- `JOB_CLEANUP_CRON` (default `@daily`): cron spec for the cleanup schedule. With multiple worker replicas, only one should run the scheduler to avoid duplicate cleanup enqueues.
