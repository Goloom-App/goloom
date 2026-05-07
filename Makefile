APP_NAME := goloom

.PHONY: fmt tidy build test run schema frontend-install frontend-dev frontend-build frontend-lint docs-api-lint docs-api-build

fmt:
	go fmt ./...

tidy:
	go mod tidy

build: frontend-build
	go build -o bin/$(APP_NAME) ./cmd/server

test:
	go test ./...

run: frontend-build
	go run ./cmd/server

schema:
	psql "$$DATABASE_URL" -f db/schema.sql

frontend-install:
	pnpm --dir frontend install

frontend-dev:
	pnpm --dir frontend dev

frontend-build:
	pnpm --dir frontend install --frozen-lockfile
	pnpm --dir frontend build

frontend-lint:
	pnpm --dir frontend lint

docs-api-lint:
	pnpm --package=@redocly/cli dlx redocly lint docs/api/openapi.yaml

docs-api-build:
	mkdir -p docs/api/dist
	pnpm --package=@redocly/cli dlx redocly build-docs docs/api/openapi.yaml -o docs/api/dist/index.html
