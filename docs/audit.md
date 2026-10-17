# Team audit activity

Team activity records meaningful team changes. System logs and team audit events
are separate records. `GET /v1/teams/{teamID}/audit-log` requires owner/admin team
access plus token `read` scope; team-bound token isolation still applies.

## Evidence contract

Each event records team, action, target type/ID, authenticated actor snapshots,
and safe metadata. Browser sessions use `oidc`; API tokens use `api_token` with
stored token ID/name snapshots. OAuth callbacks use the signed state user after
checking current membership; callbacks never invent an API-token identity.

REST metadata contains `channel=rest`, `outcome=success`, generated `request_id`,
and allowlisted `changed_fields` names when available. MCP/chat use their own
channel. Role changes contain `role_before`/`role_after`; migration records both
teams with shared `correlation_id`, `source_team_id`, `destination_team_id`.
Recurring operations include `template_id`, `occurrence_at`, `shift_to`, or mode.
Media upload includes size only. AI-draft correlation accepts valid UUID job IDs.

Summaries are generic. Credentials, API keys, invitation bearer tokens, full
URLs, prompts, post/source bodies, and raw provider errors never enter audit
payloads. Target IDs identify objects without copying their content.

## User entry-point matrix

Paths below start with `/v1`; `T` means `/teams/{teamID}`. Each row describes
successful meaningful changes; rejected requests emit no success event.

| Entry point | Action | Target / detail |
| --- | --- | --- |
| `POST /teams` | `team.create` | New team |
| `PATCH T` | `team.update` | Team settings |
| `POST T/members` | `member.add` or `member.role_change` | Member user ID; role before/after |
| `DELETE T/members/{userID}` | `member.remove` | Removed user ID |
| `POST T/invitations` | `invitation.create` | Invitation ID; no bearer value |
| `DELETE T/invitations/{invitationID}` | `invitation.revoke` | Invitation ID |
| `POST /invitations/accept` | `invitation.accept` | Accepting user ID and role; accepting actor |
| `POST T/accounts` | `account.connect` | Account ID; provider call precedes DB transaction |
| `GET /oauth/mastodon/callback` | `account.connect` | Account ID; `channel=oauth`; validated state actor |
| `PATCH T/accounts/{accountID}` | `account.update` | Account ID |
| `DELETE T/accounts/{accountID}` | `account.disconnect` | Account ID; remote revocation precedes local deletion |
| `POST T/accounts/{accountID}/migrate` | `account.migrate_out`, `account.migrate_in` | Account ID; both teams; shared correlation |
| `POST T/posts` | `post.create` | Post ID |
| `POST T/posts/draft` | `post.create` | Post ID; `source=ai_draft`; optional UUID job ID |
| `PATCH T/posts/{postID}` | `post.update` | Post ID |
| `DELETE T/posts/{postID}` | `post.delete` | Post ID; recurring deletion skip committed together |
| `POST T/posts/{postID}/cancel` | `post.cancel` | Post ID |
| `PATCH T/posts/{postID}/versions` | `post.versions_update` | Post ID; account content field names only |
| `POST/PATCH/DELETE T/rss-feeds[/{feedID}]` | `rss_feed.create/update/delete` | Feed ID; no URL/template body |
| `POST/PATCH/DELETE T/post-templates[/{templateID}]` | `recurring.create/update/delete` | Template ID |
| `POST T/post-templates/{templateID}/skip` | `recurring.skip` or `recurring.shift` | Template ID; occurrence/shift timestamps |
| `POST T/post-templates/{templateID}/regenerate` | `recurring.regenerate` | Template ID; mode/occurrence |
| `PUT/DELETE T/profile` | `brand_profile.update/delete` | Profile ID or team ID after deletion |
| `POST/PATCH/DELETE T/campaign-formats[/{formatID}]` | `campaign_format.create/update/delete` | Format ID |
| `POST/DELETE T/style-examples[/{exampleID}]` | `style_example.create/delete` | Example ID; no content |
| `POST/PATCH/DELETE T/knowledge-sources[/{sourceID}]` | `knowledge_source.create/update/delete` | Source ID; URL fetch precedes DB transaction |
| `PUT T/ai-service-config` | `ai_config.update` | Team ID; key field name only |
| `PUT T/proactive-settings` | `proactive_settings.update` | Settings ID |
| `PUT T/external-post-monitor` | `external_monitor.update` | Team ID; enabled transition |
| `POST/DELETE /me/api-tokens[/{tokenID}]` | `api_token.create/revoke` | Token ID, only when bound to a team |
| `POST T/media` | `media.upload` | Media ID; duplicate upload returns existing item silently |
| `PATCH T/media/{mediaID}` | `media.rename` | Media ID; filename field name only |
| `DELETE T/media/{mediaID}` | `media.delete` | Media ID; file cleanup follows commit |
| `POST T/ai-trigger` | `ai_job.submit` | Job ID; manager commits job and event before dispatch |
| `POST T/ai-jobs/{jobID}/cancel` | `ai_job.cancel` | Job ID; conditional terminal claim; cancel/SSE after commit |
| `POST /admin/publish-failures/{postID}/retry` | `post.retry` | Affected post/team |
| `POST /admin/publish-failures/{postID}/acknowledge` | `post.acknowledge` | Affected post/team |
| `POST T/import-old-posts` | Import lifecycle events | Actual imported posts owned by scheduler; zero imports silent |
| `POST T/ai/chat`, `POST T/ai/confirm-action`, MCP | Shared tool events below | Executing a write tool records once; conversation/proposal itself silent |

## Shared tools

MCP and chat invoke the same transactional tool implementation. Audit failure
returns tool failure and rolls back its database change. Sinks return errors;
a missing custom sink persists through the store directly.

| Tool | Action / target |
| --- | --- |
| `create_campaign` | `campaign_format.create` / `campaign_format` |
| `create_recurring` | `recurring.create` / `recurring` |
| `create_rss_feed` | `rss_feed.create` / `rss_feed` |
| `schedule_post`, `draft_post` | `post.create` / `post` |
| `modify_post` | `post.update` / `post` |
| `delete_post` | `post.delete` / `post` |

Historical `campaign.create`/`campaign` records remain intact and readable.

## Quiet operations and exclusions

Identical membership roles, missing-member removals, same-team migrations,
repeated occurrence skip/shift, duplicate media uploads/renames, unchanged
RSS/template/team/account/profile/campaign/monitor settings, unchanged account
versions, and cancellation of an already failed job emit no new success.
Completed-job cancellation is rejected; a concurrent completion cannot be
replaced with a false cancellation event.

Reads, validation, prompt previews, discovery, OAuth-start redirects, chat-only
conversation/proposals, SSE, poll cursors, routine metrics refresh, and empty
imports have no team activity. Admin sync triggers dispatch work; actual changes
are recorded by workers. Development-only E2E/demo seeding is test setup.

Global user/provider-instance administration, system-log archive/prune, OIDC and
session authentication, personal unbound tokens, push subscriptions/preferences,
and tour state remain global/personal/security concerns. They are not copied to
arbitrary teams. Admin post repair changes team posts and belongs in lifecycle
coverage, not this exclusion list.

## Database boundary and regression checks

`API.auditedHandler` buffers HTTP headers/status/body until `WithAuditTransaction`
commits both mutation and evidence. Errors and request cancellation roll back
both. Provider/OAuth/media/URL-fetch handlers put only their database tail inside
that boundary. Worker dispatch, notifications, and in-process cancellation use
`store.AfterAuditCommit` with its transaction-free context.

Regression suites cover read scope; REST actor/team/target metadata; RSS and
recurring CRUD; roles and no-ops; both migration teams; invitation acceptance;
validated OAuth attribution; secret exclusion; audit insert failure; request
cancellation; concurrent AI completion/cancellation; and shared-tool rollback and
canonical campaign semantics.
