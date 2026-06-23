# Post creation & validation pipeline

Every interactive way to create or update a post — the REST API and the MCP
server — runs through a single pipeline so the two cannot drift apart. A gap in
the pipeline is a gap everywhere; a new caller cannot accidentally skip a step.

## The one entry point: `internal/postservice`

`postservice.Service.Prepare(ctx, teamID, input, opts)` does, in order:

1. **Normalize** — `domain.CreatePostInput.Normalize()`: trims title/content,
   normalizes visibility, dedupes media ids, scopes media exclusions and
   per-account overrides to the post, and derives `UseVersions`.
2. **Validate shape** — `domain.CreatePostInput.Validate()`: a **title is always
   required**; scheduled posts also require content and at least one target.
3. **Resolve targets** — `Service.ResolveTargets`: every target account must
   exist and belong to the team; every `account_content_override` key must
   reference a target (a misdirected override is an error, never silently
   dropped). `Options.RequireTeam` rejects an empty team so a post is never
   stored with an inferred/empty team.
4. **Check destinations** — `postvalidate.Check` (only when
   `Options.CheckLimits`, i.e. not for drafts): per-account character limits and
   media requirements, honoring overrides. Reported softly in
   `Result.Validation` so the preview endpoint can show it; mutating callers
   treat `!Valid` as a rejection.

`postvalidate` is the single source of truth for character/media limits, shared
by REST, MCP and the AI chat pre-check (`CheckLimits`).

## Callers

- **REST**: `handleCreatePost`, `handleUpdatePost`, `handleValidatePost`
  (preview, no `RequireTeam`).
- **MCP**: `schedule_post`, `draft_post`, `modify_post`; `create_recurring` /
  `create_rss_feed` reuse `ResolveTargets` (templates/feeds have targets but no
  post body).
- **Automation** (RSS/AI/recurring callbacks, template materialization, review
  queue): these build posts from internal state and call `EnsureTitle` to
  generate a title instead of requiring one — "required by default, generated
  where necessary". They are the only paths allowed to bypass `Prepare`, and are
  explicitly listed in `postservice.automationBypassAllowlist`.

## Guardrails

- `internal/postservice/guard_test.go` (`TestNoUnvalidatedPostCreation`) fails the
  build if a non-allowlisted file calls `store.CreateScheduledPost` without going
  through `Prepare`.
- `internal/postservice/contract_test.go` (`TestContract`) is the readable
  specification of post validity.

## Storage

`store.CreateScheduledPost` is pure persistence. Field resolution shared by the
sqlite and postgres backends (author, status, source, template/feed references)
lives in `domain.ResolvePostInsert` so the two backends only differ in SQL and
time encoding.

## Client note

Because a title is now required, the web UI must send a non-empty `title` when
creating a post; otherwise the API responds `400`.
