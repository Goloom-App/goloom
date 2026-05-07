# Account & team model — operator checklist

Concise checklist for personal teams, account migration, invitations, and backward-compatible account routing. **Not** an implementation spec.

---

## 1) Migration steps (operators)

- **Schema**
  - Apply migrations that add `teams.is_personal`, `teams.personal_for_user_id` (unique where set), and the `team_invitations` (or equivalent) table with indexes and FKs as defined in repo migrations.
  - Confirm `social_accounts` constraints still match product rules after team moves (cascade behavior, uniqueness per provider if any).

- **Backfill**
  - Run the one-off data migration that creates a personal team per existing user and attaches their standalone or legacy-owned accounts as appropriate.
  - Record counts before/after: users, teams, `social_accounts` by `team_id`, orphaned rows.

- **Deploy order**
  - Prefer: migrate DB → deploy app version that calls `EnsurePersonalTeam` on user creation and uses new auth resolution → enable any feature flags if applicable.
  - If downtime window exists, communicate API behavior change: URLs with stale `teamID` may still work when account ID is authoritative.

- **Verification**
  - Spot-check DB: each user has at most one personal team; `personal_for_user_id` matches owner; no duplicate personal teams per user.
  - Smoke-test login, create user, list teams/accounts, schedule post, invitation accept on staging.

- **Rollback**
  - Know whether schema rollback is supported; if not, plan forward-fix only. Keep backup before destructive steps.

---

## 2) Test cases (store + HTTP)

### Store

- New user path: user insert triggers exactly one personal team; idempotent `EnsurePersonalTeam` if called twice.
- Migrate account: `social_accounts.team_id` updates from personal team to target team; membership/role on target team enforced by store API.
- **Scheduled posts**: posts tied to the account remain valid or are rejected/cleaned up per rules when `team_id` on the account changes (no dangling `team_id` on posts vs account).
- Invitations: create with expiry; accept consumes or marks used; duplicate/expired/revoked invites behave as specified.
- Team uniqueness: cannot create a second personal team for the same user; `personal_for_user_id` invariant.

### HTTP / API

- Account-scoped routes: access granted when session user may act on resolved `social_accounts.id`; **wrong `teamID` in path** still succeeds if the account belongs to another team the user can access (backward compatibility).
- Routes that still require path `teamID` to match true team: document and test any exceptions (if any endpoints intentionally strict).
- Migrate-account POST: rejects unauthorized target team, non-member, or account not owned by caller’s personal context as per rules.
- Invitation endpoints: create (permission-gated), list pending, accept with token/body, idempotent accept.

---

## 3) Security checks

### IDOR & cross-team access

- Never authorize using path `teamID` alone; always resolve `social_accounts.id` (and post IDs) and check membership/role on the **actual** team row.
- User A cannot migrate or attach accounts to teams they do not belong to or lack admin/migrate permission on.
- Listing endpoints do not leak accounts or posts from teams the user is not in (including after team change).

### Invite token

- Token is unguessable (length/entropy); single-use or explicitly idempotent with clear semantics.
- Expired and revoked invites cannot be accepted; accepting for wrong email/user identity fails closed.
- Rate-limit or throttle accept attempts if brute-force is plausible.

### Role gates

- Only roles allowed to **create invites** and **migrate accounts** can do so (assert in handler + store).
- Personal-team-only operations (e.g. “leave” personal team) denied or undefined per product; ensure no privilege escalation via personal team edge cases.
