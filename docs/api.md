# ScholarFlow Server API

## Endpoints

- `GET /healthz`
- `POST /v1/uploads/papers`
- `GET /v1/jobs/{id}`
- `GET /v1/papers/{id}`

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
