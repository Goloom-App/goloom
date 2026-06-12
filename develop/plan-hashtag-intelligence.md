# Hashtag Analytics & Intelligence Plan

## Objective
Enable data-driven hashtag strategies by correlating hashtag usage with post performance. Provide users (Analytics view, Composer) and the AI Assistant with hashtag suggestions based on historical engagement.

## Verified Findings (2026-06-12)

- **Case sensitivity:** Hashtags are case-insensitive on all three connected platforms (Mastodon normalizes tags for search/grouping, Friendica is Mastodon-compatible, Bluesky matches tags case-insensitively in feeds/search). → Group by a **lowercase-normalized key**, keep a **display variant** (CamelCase tags matter for accessibility).
- **No "reach" metric exists:** none of the provider APIs expose impressions. The honest proxy is the engagement score (likes + reposts + replies [+ quotes on Bluesky]) already collected in `post_metrics` / `post_metrics_history`. UI labels must say "engagement", not "reach".
- **Bluesky facet gap (prerequisite fix):** `internal/provider/bluesky.go` publishes plain text without `app.bsky.richtext.facet`. Hashtags and links in Goloom posts are **not clickable and not indexed in Bluesky tag feeds** — they currently generate no hashtag-driven reach there. Must be fixed first, otherwise hashtag analytics measures structural zeros on Bluesky.
- **Engagement-hours histogram already exists** (`GET /v1/teams/{teamID}/analytics/engagement-hours`, used by Composer `ScheduleInsights` and `GetTeamAIContext`) but is hour-of-day (UTC) only and absent from the Analytics view.
- Hashtags are not extracted anywhere in the codebase today; per-account content can differ via `post_versions`, and metrics are keyed `(post_id, account_id, metric)`.

## Architecture Decisions

1. **Join table, not pre-aggregated stats.** `post_hashtags(post_id, account_id, tag_norm, tag_display)` populated at publish time. Metrics keep changing after publish via the sync job; pre-aggregated sums (the earlier `hashtag_stats` idea) would need constant idempotent updates. A join against `post_metrics` yields live numbers, time windows, and per-platform filters for free.
2. **Per-account rows.** Content differs per account (`post_versions`), and `post_metrics` is per account — joining on `(post_id, account_id)` keeps platform attribution exact.
3. **Score design.** A raw sum lets one viral post dominate. Report per tag: `uses`, `total_engagement`, `avg_engagement = total/uses`, and rank by a smoothed score `total / (uses + k)` (k=3) so single-use outliers don't auto-win. Always display the usage count next to the score.
4. **Extraction:** Unicode-aware, `#[\p{L}\p{N}_]+` (German umlauts!), strip leading `#`, normalize with `strings.ToLower` (Unicode-aware enough for tags; platforms do ASCII-ish folding, lowercasing via `strings.ToLower` matches Mastodon behavior closely). Display variant = casing of the most recent use.

## Implementation Steps

### 0. Bluesky rich text facets (prerequisite)
- `internal/provider/bluesky.go`: build `facets` for the post record — `app.bsky.richtext.facet#tag` for hashtags, `#link` for URLs. Byte offsets are **UTF-8 byte** indices (`byteStart`/`byteEnd`).
- Unit tests with umlauts/emoji before the tag to pin offset math.

### 1. Extraction package
- `internal/hashtag`: `Extract(content string) []Tag` with `Tag{Norm, Display string}`, dedup per post, skip tags inside URLs.

### 2. Schema (postgres + sqlite, both idempotent schema.sql)
```sql
create table if not exists post_hashtags (
    post_id    <uuid/text> not null references scheduled_posts(id) on delete cascade,
    account_id <uuid/text> not null references social_accounts(id) on delete cascade,
    tag_norm   text not null,
    tag_display text not null default '',
    primary key (post_id, account_id, tag_norm)
);
create index if not exists idx_post_hashtags_tag on post_hashtags(tag_norm);
```

### 3. Population
- Store method `ReplacePostHashtags(ctx, postID, accountID, tags)` (delete + insert, both stores).
- Scheduler `processPost`: after successful publish per account, extract from the final expanded per-account content and store.
- **Backfill:** on app start, `BackfillPostHashtags` walks posted posts (joined with `post_versions` fallback to `content`) and inserts with on-conflict-do-nothing. Idempotent, small data volume.

### 4. Analytics API
- Store `ListTeamHashtagPerformance(ctx, teamID string, days int, provider string, limit int) []HashtagPerformance` — join `post_hashtags × post_metrics × scheduled_posts(status='posted') × social_accounts(provider)`.
- `GET /v1/teams/{teamID}/analytics/hashtags?days=90&provider=&limit=30` (viewer+).

### 5. Activity heatmap (weekday × hour)
- Extend histogram query with `extract(dow ...)`: new bucket `{weekday, hour, score}`, endpoint param `?by=weekday-hour` on the existing engagement-hours route (UTC buckets; frontend converts to local time).
- Analytics view: new "Activity" panel with the heatmap; answers "when is the platform most active".

### 6. Frontend
- `api.ts`: `getTeamHashtagPerformance`, extended engagement-hours client.
- AnalyticsView: new "Hashtags" tab — table: tag (display), uses, total engagement, Ø engagement, score; platform filter; plus the Activity heatmap on Overview.
- Composer: typing `#` opens suggestion chips (top tags by score, prefix-filtered); selecting inserts the display variant.

### 7. AI integration
- `domain.AIContext` + `GetTeamAIContext`: add `TopHashtags []HashtagPerformance` (top 20, 90d).
- Prompt rendering: compact list "tag (uses, avg engagement)" with the instruction that picking none is valid.
- Chat tool `get_top_hashtags` in `api/ai_chat.go` (params: days, provider, limit).
- MCP tool `get_hashtag_performance` in `internal/mcp`.

## Verification
- Unit: extraction (umlauts, dedup, URL exclusion), facet byte offsets, store queries (postgres + sqlite test suites).
- `go test ./...`, frontend build, e2e smoke.
- Manual: publish post with mixed-case tags on two accounts → analytics groups them under one normalized tag; Bluesky post shows clickable tags.
