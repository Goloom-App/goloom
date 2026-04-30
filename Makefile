APP_NAME := goloom

.PHONY: fmt tidy build test run schema

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
