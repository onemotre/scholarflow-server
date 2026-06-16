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

## Worker Parse Pipeline

The `paper:process` worker task currently runs the parse-only pipeline:

1. Marks the processing job `processing`.
2. Loads the uploaded PDF asset from object storage.
3. Calls the configured parser. The first parser is GROBID.
4. Stores raw GROBID TEI XML as a `grobid_tei_xml` asset.
5. Persists parsed metadata, authors, sections, and references.
6. Marks the job `parsed`.

If parsing or persistence fails, the job is marked `failed` and the worker returns the error to asynq for retry handling.
