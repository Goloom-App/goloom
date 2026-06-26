# GHCR mirror of Docker Hub (see https://github.com/psarossy/dockerhub-mirror).
# Pin to the build host's platform so multi-arch builds compile once natively and
# the Go stage cross-compiles per target arch (no qemu emulation).
FROM --platform=$BUILDPLATFORM ghcr.io/psarossy/node:lts AS frontend-builder

WORKDIR /src

COPY frontend/package.json frontend/pnpm-lock.yaml ./frontend/
COPY frontend ./frontend
# Explicit files: empty locales/ in build context otherwise passes COPY but tsc exits 2.
COPY locales/en.json locales/de.json ./locales/
RUN mkdir -p /src/internal/webui && \
    test -s locales/en.json && test -s locales/de.json || \
      (echo "ERROR: locales/en.json and locales/de.json missing or empty in Docker build context" >&2; ls -la locales/ 2>&1 || true; exit 1) && \
    # Stale-file sweep: some deploy tools (Dockhand) cp without --delete, so removed
    # source files reappear in the build context. Drop known-removed files explicitly.
    rm -f frontend/src/views/ai/ProactiveTriggersView.tsx \
          frontend/src/views/recurring/RecurringPostsView.tsx.bak \
          frontend/src/views/rss/RSSFeedsView.tsx.bak && \
    corepack enable && \
    (corepack prepare pnpm@10.33.0 --activate || \
      (curl -fsSL https://github.com/pnpm/pnpm/releases/download/v10.33.4/pnpm-linux-x64 -o /usr/local/bin/pnpm && \
       chmod +x /usr/local/bin/pnpm)) && \
    pnpm --version && \
    CI=true pnpm --dir frontend install --frozen-lockfile && \
    pnpm --dir frontend build

# go.mod requires go 1.26.4 (security bump). The psarossy mirror only carries up
# to 1.26.3, and that image pins GOTOOLCHAIN=local, so an in-build toolchain
# upgrade is blocked — use the official Docker Hub image that ships 1.26.4
# directly (the runner already pulls Docker Hub library images, e.g. postgres).
FROM --platform=$BUILDPLATFORM golang:1.26.4-alpine3.22 AS builder

WORKDIR /src

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=frontend-builder /src/internal/webui/dist ./internal/webui/dist
# Release version embedded into the binary (see internal/version). Defaults to
# "dev"; CI passes the real version via --build-arg VERSION=vX.Y.Z.
ARG VERSION=dev
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
	go build -ldflags "-X git.f4mily.net/goloom/internal/version.Version=${VERSION}" \
	-o /out/goloom ./cmd/server

FROM gcr.io/distroless/static-debian12

WORKDIR /app

COPY --from=builder /out/goloom /app/goloom

# Defaults for container deployments (override in compose / k8s as needed).
ENV APP_ENV=production

EXPOSE 8080

ENTRYPOINT ["/app/goloom"]
