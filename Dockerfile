FROM node:22 AS frontend-builder

WORKDIR /src

COPY frontend/package.json frontend/pnpm-lock.yaml ./frontend/
COPY frontend ./frontend
RUN mkdir -p /src/internal/webui && corepack enable && CI=true pnpm --dir frontend install --frozen-lockfile && pnpm --dir frontend build

FROM golang:1.26 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=frontend-builder /src/internal/webui/dist ./internal/webui/dist
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/goloom ./cmd/server

FROM gcr.io/distroless/static-debian12

WORKDIR /app

COPY --from=builder /out/goloom /app/goloom

EXPOSE 8080

ENTRYPOINT ["/app/goloom"]
