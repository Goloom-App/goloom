# Automation AI preview (#161)

## Acceptance

- Editor/owner selects real RSS entry or concrete recurring main/announcement occurrence.
- Preview shows source, expanded template, effective generation inputs, generated text per account, and character counts.
- Generation uses normal prompt preparation and target constraints.
- Preview job remains identifiable and never creates posts, schedules, RSS markers, or counter changes on success, failure, cancellation, timeout, or recovery.
- Backend integration and browser tests cover both automation types and side effects.

## Interfaces

- `GET /v1/teams/{teamID}/rss-feeds/{feedID}/preview-items` lists bounded real feed entries.
- `POST /v1/teams/{teamID}/rss-feeds/{feedID}/ai-preview` accepts `item_key`.
- `POST /v1/teams/{teamID}/post-templates/{templateID}/ai-preview` accepts `occurrence_at` and `post_kind`.
- Existing AI job GET supplies terminal result and input snapshot.

## Security

- Require AI-enabled team and editor/owner before source lookup or outbound calls.
- Load feeds/templates and target accounts by team; guard on-demand RSS fetch against SSRF.
- Keep preview intent server-owned, omit automation completion metadata, and set non-publishing intent.
- Return fixed errors and keep feed URLs/item keys out of logs.

## Slices

1. Shared preparation and RSS entry selection; tests for parity and boundaries.
2. Preview job submission and terminal safety; tests for success and failures.
3. RSS/recurring UI, job results, localized copy, and Playwright coverage.
