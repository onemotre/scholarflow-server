.PHONY: test fmt up dev deps-up deps-down migrate

test:
	go test ./...

fmt:
	go fmt ./...

# Build and start the full server stack (api, worker, postgres, redis, minio,
# grobid) in the background. Schema migrations run automatically at startup.
up:
	docker compose up -d --build

# Same as `up` but stays attached and streams logs.
dev:
	docker compose up --build

deps-up:
	docker compose up postgres redis minio grobid

deps-down:
	docker compose down

migrate:
	goose -dir migrations postgres "$$DATABASE_URL" up
