APP_NAME := goloom

.PHONY: fmt tidy build test run schema frontend-install frontend-dev frontend-build frontend-lint

fmt:
	go fmt ./...

tidy:
	go mod tidy

build:
	go build -o bin/$(APP_NAME) ./cmd/server

test:
	go test ./...

run:
	go run ./cmd/server

schema:
	psql "$$DATABASE_URL" -f db/schema.sql

frontend-install:
	pnpm --dir frontend install

frontend-dev:
	pnpm --dir frontend dev

frontend-build:
	pnpm --dir frontend build

frontend-lint:
	pnpm --dir frontend lint
