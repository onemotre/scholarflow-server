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
