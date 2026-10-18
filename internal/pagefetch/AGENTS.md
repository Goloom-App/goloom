# internal/pagefetch

## Purpose

Fetch public web pages for API source URLs and optional RSS article enrichment.

## Ownership

`pagefetch.go` owns guarded HTTP retrieval and text extraction; `github.go` rewrites GitHub web links to raw text candidates.

## Local Contracts

- Validate original and candidate URLs with `netguard.Policy.CheckURL`; `HTTPClient` guards redirects and actual dial. Production callers use a public-only policy.
- Bound total operation by caller timeout and response body by 2 MiB; reject larger or non-text responses. `htmltext.ExtractReadableText` caps extracted text at 12,000 characters.
- Return errors for empty readable text so callers can expose fallback; never put raw URL errors or credentials in user-facing notices.

## Work Guidance

- Keep network calls outside database transactions.

## Verification

- `go test ./internal/pagefetch/...`

## Child DOX Index

None.
