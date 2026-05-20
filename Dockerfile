# GHCR mirror of Docker Hub (see https://github.com/psarossy/dockerhub-mirror).
FROM ghcr.io/psarossy/node:lts AS frontend-builder

WORKDIR /src

COPY frontend/package.json frontend/pnpm-lock.yaml ./frontend/
COPY frontend ./frontend
COPY locales ./locales
RUN mkdir -p /src/internal/webui && corepack enable && corepack prepare pnpm@10.33.0 --activate && CI=true pnpm --dir frontend install --frozen-lockfile && pnpm --dir frontend build

FROM ghcr.io/psarossy/golang:1.26.2-alpine3.22 AS builder

WORKDIR /src

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=frontend-builder /src/internal/webui/dist ./internal/webui/dist
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/goloom ./cmd/server

FROM gcr.io/distroless/static-debian12

WORKDIR /app

COPY --from=builder /out/goloom /app/goloom

# Defaults for container deployments (override in compose / k8s as needed).
ENV APP_ENV=production

EXPOSE 8080

ENTRYPOINT ["/app/goloom"]
