# GHCR mirror of Docker Hub (see https://github.com/psarossy/dockerhub-mirror).
FROM ghcr.io/psarossy/node:lts AS frontend-builder

WORKDIR /src

COPY frontend/package.json frontend/pnpm-lock.yaml ./frontend/
COPY frontend ./frontend
# Explicit files: empty locales/ in build context otherwise passes COPY but tsc exits 2.
COPY locales/en.json locales/de.json ./locales/
RUN mkdir -p /src/internal/webui && \
    test -s locales/en.json && test -s locales/de.json || \
      (echo "ERROR: locales/en.json and locales/de.json missing or empty in Docker build context" >&2; ls -la locales/ 2>&1 || true; exit 1) && \
    corepack enable && \
    (corepack prepare pnpm@10.33.0 --activate || \
      (curl -fsSL https://github.com/pnpm/pnpm/releases/download/v10.33.4/pnpm-linux-x64 -o /usr/local/bin/pnpm && \
       chmod +x /usr/local/bin/pnpm)) && \
    pnpm --version && \
    CI=true pnpm --dir frontend install --frozen-lockfile && \
    pnpm --dir frontend build

FROM ghcr.io/psarossy/golang:1.26.3-alpine3.22 AS builder

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
