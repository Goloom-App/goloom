# internal/provider

## Purpose

Social media provider integrations. Each provider (Bluesky, Friendica, Mastodon) implements the Provider interface for posting, media upload, metrics fetching, and author feeds.

## Ownership

Provider interface in `provider.go`, registry pattern. Individual implementations per provider.

## Local Contracts

- Interface: `provider.go` — abstract provider contract
- Registry: `provider.go` — provider registration and lookup
- Bluesky: `bluesky.go` + `bluesky_*.go` files
- Friendica: `friendica.go`
- Mastodon: `mastodon.go` + `mastodon_*.go` files
- Shared utilities: `utils.go`, `instance_host.go`, `remote_post_id.go`
- Metrics: `metrics_url.go`, provider-specific metrics files
- Author feeds: `author_post_ids.go`, provider-specific feed files
- Outbound policy: `outbound_policy.go`

## Work Guidance

- New providers must implement the Provider interface
- Register in provider registry on init
- Each provider has distinct API patterns — document provider-specific quirks
- Media upload follows provider-specific APIs
- Metrics fetching is async and provider-specific
- Test with provider mocks where possible
- Instance URL resolution in `instance_host.go`

## Verification

- `go test ./internal/provider/...` must pass
- Instance host tests in `instance_host_test.go`
- Author post ID tests in `author_post_ids_test.go`

## Child DOX Index

None — flat package with per-provider file groups.
