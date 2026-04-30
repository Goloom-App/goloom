FROM golang:1.26 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/goloom ./cmd/server

FROM gcr.io/distroless/static-debian12

WORKDIR /app

COPY --from=builder /out/goloom /app/goloom
COPY db/schema.sql /app/db/schema.sql

EXPOSE 8080

ENTRYPOINT ["/app/goloom"]
