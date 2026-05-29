APP_NAME := goloom

.PHONY: fmt tidy build test run schema frontend-install frontend-dev frontend-build frontend-lint frontend-e2e docs-api-lint docs-api-build ai-service-build ai-service-run ai-service-test

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

# End-to-end UI tests (Playwright): installs browsers, builds UI + server, runs tests (see frontend/package.json test:e2e).
frontend-e2e:
	pnpm --dir frontend install --frozen-lockfile
	pnpm --dir frontend exec playwright install chromium
	pnpm --dir frontend test:e2e

docs-api-lint:
	pnpm --package=@redocly/cli dlx redocly lint docs/api/openapi.yaml

docs-api-build:
	mkdir -p docs/api/dist
	pnpm --package=@redocly/cli dlx redocly build-docs docs/api/openapi.yaml -o docs/api/dist/index.html

ai-service-build:
	cd ai-service && uv sync --dev

ai-service-run:
	cd ai-service && uv run uvicorn app.main:app --host 0.0.0.0 --port 8090

ai-service-test:
	cd ai-service && uv run python -m pytest tests/ -v
