# goloom developer tasks — run `just` inside `nix develop` for the full toolchain.
# Recipes stay thin and delegate to the Makefile / the real tools, so CI
# (.forgejo/workflows/ci.yaml) and local runs use the same commands.

# show available recipes
default:
    @just --list

# install/refresh all dependencies (Go modules + frontend)
setup:
    go mod download
    pnpm --dir frontend install --frozen-lockfile

# build the production binary (embeds the frontend)
build:
    make build

# run the server locally with the embedded frontend (uses .env)
run:
    make run

# frontend dev server with hot reload (Vite, expects a running backend)
dev-frontend:
    make frontend-dev

# throwaway live instance on http://127.0.0.1:8080 — fresh DB, dev-only secrets
try: build
    mkdir -p .e2e && rm -f .e2e/try.db .e2e/try.db-wal .e2e/try.db-shm
    @echo "sign in with token: local-dev-only-bootstrap-token-32chars (data: .e2e/try.db)"
    APP_ENV=development \
    HTTP_ADDR=127.0.0.1:8080 \
    DATABASE_URL="file:.e2e/try.db?_journal_mode=WAL" \
    ENCRYPTION_KEY=local-dev-only-encryption-key-32chars \
    BOOTSTRAP_ADMIN_TOKEN=local-dev-only-bootstrap-token-32chars \
    ./bin/goloom

# Go unit tests (sqlite store included; postgres suite needs test-postgres)
test:
    go test ./...

# Postgres store integration tests in a throwaway container
test-postgres:
    make test-postgres

# full backend test surface (unit + postgres)
test-all: test test-postgres

# Playwright end-to-end suite (builds UI + server first)
e2e:
    make build
    make frontend-e2e

# a single e2e spec, e.g. `just e2e-spec onboarding`
e2e-spec spec: build
    pnpm --dir frontend exec playwright test e2e/{{spec}}.spec.ts

# linters: go vet + eslint
lint:
    go vet ./...
    make frontend-lint

# gofmt over the Go tree
fmt:
    make fmt

# coverage report (total must not drop — see AGENTS.md)
cover:
    make cover

# security scans as CI runs them (govulncheck + gosec, high severity blocks)
security:
    go run golang.org/x/vuln/cmd/govulncheck@latest ./...
    go run github.com/securego/gosec/v2/cmd/gosec@latest -severity high ./...

# local mirror of the CI gate: build, vet, tests, frontend lint + build
check: lint test
    go build ./...
    pnpm --dir frontend build

# OpenAPI spec lint (docs/api/openapi.yaml)
docs-lint:
    make docs-api-lint

# docs website with live reload (Astro/Starlight)
website:
    make website-dev
