# GHCR mirror of Docker Hub (see https://github.com/psarossy/dockerhub-mirror).
FROM ghcr.io/psarossy/node:lts AS frontend-builder

WORKDIR /src

COPY frontend/package.json frontend/pnpm-lock.yaml ./frontend/
COPY frontend ./frontend
# Explicit locale copy: frontend imports @locales/* from frontend/locales (symlink in dev).
COPY locales/en.json locales/de.json ./frontend/locales/
ARG TARGETARCH=amd64
RUN mkdir -p /src/internal/webui /src/frontend/locales && \
    test -f frontend/locales/en.json && test -f frontend/locales/de.json
RUN /bin/bash -eu -o pipefail <<'BASH'
corepack enable
PNPM_VERSION="$(node -p "require('./frontend/package.json').packageManager.split('@')[1]")"
if ! corepack prepare "pnpm@${PNPM_VERSION}" --activate; then
  echo "corepack prepare failed; installing pnpm ${PNPM_VERSION} from GitHub release" >&2
  case "${TARGETARCH}" in
    arm64) PNPM_ARCH=arm64 ;;
    *) PNPM_ARCH=x64 ;;
  esac
  curl -fsSL "https://github.com/pnpm/pnpm/releases/download/v${PNPM_VERSION}/pnpm-linux-${PNPM_ARCH}" \
    -o /usr/local/bin/pnpm
  chmod +x /usr/local/bin/pnpm
fi
pnpm --version
CI=true pnpm --dir frontend install --frozen-lockfile
BASH
RUN pnpm --dir frontend build

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
