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
- Threads: optional `ThreadFetcher` (`provider.go`, type-asserted like `AuthorFeedFetcher`); Mastodon + Friendica share `fetchMastodonCompatibleThread` in `mastodon_thread.go` (`/statuses/{id}/context` descendants, direct-visibility dropped, max 200, body capped 4 MiB, plain-text via `htmlToPlainText`). `ThreadTarget.RemoteID` (stored `remote_post_id`, via `store.GetPublishedTarget`) is preferred; the numeric id is parsed from `PublishedURL` only as fallback (Friendica `/display/<guid>` has none). Bluesky not implemented (use `ThreadTarget.Metadata` AT-URI)
- Outbound policy: `outbound_policy.go`

## Work Guidance

- New providers must implement the Provider interface
- Register in provider registry on init
- Each provider has distinct API patterns — document provider-specific quirks
- Media upload follows provider-specific APIs
- Metrics fetching is async and provider-specific
- Test with provider mocks where possible
- Instance URL resolution in `instance_host.go`
- Publish HTTP 4xx rejections (except 408) use `PublicationRejectedError`. Transport errors, 408, 5xx and malformed success responses retain uncertain outcomes; callers must not automatically republish them.

## Verification

- `go test ./internal/provider/...` must pass
- Instance host tests in `instance_host_test.go`
- Author post ID tests in `author_post_ids_test.go`

## Child DOX Index

None — flat package with per-provider file groups.
