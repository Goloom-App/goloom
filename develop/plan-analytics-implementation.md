# Plan: Analytics & Metrics Implementation

This plan details the steps to fully realize the performance tracking and data-driven insights described in `docs/analytics-parameters.md`.

## 1. Backend: Data Collection & Storage

### A. Database Migration (`db/schema.sql`)
The current `post_metrics` table only stores the latest values. To support historical analysis and deltas, we need to transition to a snapshot-based approach.

```sql
-- Rename or modify current table to support daily snapshots
create table if not exists post_metrics_history (
    post_id uuid not null references scheduled_posts(id) on delete cascade,
    account_id uuid not null references social_accounts(id) on delete cascade,
    metric text not null,
    value bigint not null default 0,
    recorded_at date not null default current_date,
    primary key (post_id, account_id, metric, recorded_at)
);
```

### B. Scheduler Enhancement (`internal/scheduler/scheduler.go`)
- **Metric Sync Loop:** Refine `syncPostedMetrics` to ensure it captures daily snapshots instead of just overwriting a single record.
- **Provider Methods:** Ensure `MastodonProvider.GetMetrics` and other future providers return the standard parameters defined in the catalog (likes, reblogs, etc.).

### C. Analysis Logic (`internal/store/`)
- **Delta Calculation:** Implement SQL queries or Go logic to compare the latest snapshot with the previous day's snapshot to calculate 24h growth.
- **Aggregation:** Implement workspace-level (team-level) aggregation to sum metrics across all accounts in a given time period.

## 2. API Endpoints (`api/http.go`)

Implement the following endpoints:
- `GET /v1/teams/{teamID}/analytics/summary`: Returns total reach, engagement, and follower growth with delta percentages.
- `GET /v1/teams/{teamID}/analytics/posts`: Returns a list of posts sorted by performance metrics.
- `GET /v1/teams/{teamID}/analytics/chart`: Returns time-series data for a specific metric (e.g., total likes over 30 days).

## 3. Frontend: UI & Visualization

### A. Analytics View (`frontend/src/views/Analytics/`)
- **Summary Tiles:** Large cards showing "Total Engagement", "Total Shares", etc., with green/red indicator for deltas.
- **Chart Components:** Use `Recharts` to display:
    - Line chart of engagement over time.
    - Bar chart comparing performance between different platforms (Mastodon vs. BlueSky).
- **Post Ranking Table:** A sortable table of all published posts showing their individual metrics.

### B. Post-Detail Analytics
- Add an "Analytics" tab or section to the expanded post view in the calendar/archive to show the performance of that specific post.

## 4. Advanced Analysis (Optimization)

- **Best Time to Post:** Implement logic that correlates high engagement values with the `scheduled_at` hour of the day.
- **UI Tooltip:** In the Composer, add a "Suggested Time" badge based on this analysis.

## 5. Implementation Roadmap

1.  **Phase 1 (DB & Storage):** Create the `post_metrics_history` table and update the `UpsertPostMetrics` store method to use daily snapshots.
2.  **Phase 2 (Aggregation API):** Develop the backend endpoints for summary and chart data.
3.  **Phase 3 (Frontend Framework):** Add `recharts` dependency and build the basic Analytics page layout with summary tiles.
4.  **Phase 4 (Deep Analysis):** Implement the "Best Time to Post" logic and advanced chart filters (e.g., filter by account/provider).
