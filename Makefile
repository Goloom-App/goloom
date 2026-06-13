APP_NAME := goloom

.PHONY: fmt tidy build test test-postgres cover run schema frontend-install frontend-dev frontend-build frontend-lint frontend-e2e docs-api-lint docs-api-build website-spec website-install website-dev website-build website-screenshots

fmt:
	go fmt ./...

tidy:
	go mod tidy

build: frontend-build
	go build -o bin/$(APP_NAME) ./cmd/server

test:
	go test ./...

# Prefer docker when its daemon is reachable, otherwise podman.
CONTAINER_RUNTIME := $(shell docker info >/dev/null 2>&1 && echo docker || echo podman)

# Postgres integration tests: throwaway Postgres container on port 15432,
# torn down afterwards. The postgres store tests skip without TEST_POSTGRES_URL.
test-postgres:
	$(CONTAINER_RUNTIME) run -d --rm --name goloom-test-pg \
		-e POSTGRES_PASSWORD=test -e POSTGRES_DB=goloom_test \
		-p 15432:5432 docker.io/library/postgres:16-alpine
	@until $(CONTAINER_RUNTIME) exec goloom-test-pg pg_isready -U postgres >/dev/null 2>&1; do sleep 0.5; done
	@TEST_POSTGRES_URL="postgres://postgres:test@localhost:15432/goloom_test?sslmode=disable" \
		go test ./internal/store/postgres/; \
		status=$$?; $(CONTAINER_RUNTIME) rm -f goloom-test-pg >/dev/null; exit $$status

cover:
	go test ./... -coverprofile=coverage.out -covermode=count
	@go tool cover -func=coverage.out | tail -1

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

# Vendor the Redoc standalone bundle into the website's public dir at build time
# so the /api/ page renders the reference fully same-origin (no runtime CDN).
REDOC_VERSION := v2.5.3
docs-api-build:
	mkdir -p website/public/api
	curl -fsSL https://cdn.redocly.com/redoc/$(REDOC_VERSION)/bundles/redoc.standalone.js \
		-o website/public/api/redoc.standalone.js

# Copy the OpenAPI spec into the website's public dir so it is downloadable at
# /openapi.yaml.
website-spec:
	mkdir -p website/public
	cp docs/api/openapi.yaml website/public/openapi.yaml

website-install:
	pnpm --dir website install

website-dev: website-spec docs-api-build
	pnpm --dir website dev

website-screenshots: frontend-build
	go build -o bin/goloom ./cmd/server
	pnpm --dir frontend install --frozen-lockfile
	pnpm --dir frontend exec playwright install chromium
	pnpm --dir frontend exec playwright test e2e/website-screenshots.spec.ts

website-build: website-spec docs-api-build
	pnpm --dir website install --frozen-lockfile
	pnpm --dir website build
