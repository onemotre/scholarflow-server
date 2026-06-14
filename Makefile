.PHONY: test fmt dev deps-up deps-down migrate

test:
	go test ./...

fmt:
	go fmt ./...

dev:
	docker compose up

deps-up:
	docker compose up postgres redis minio grobid

deps-down:
	docker compose down

migrate:
	goose -dir migrations postgres "$$DATABASE_URL" up
