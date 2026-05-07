"Advanced Analytics," goloom needs to move from purely post-centric tracking to include account-level growth tracking.

  1. Extend Provider Interface & Implementations
  Add support for account-level data fetching in the core provider interface.
   * Update SocialMediaProvider interface in internal/provider/provider.go:

   1     GetAccountMetrics(ctx context.Context, account domain.SocialAccount, auth PublishAuth) ([]EngagementMetric, error)
   * Mastodon implementation: Fetch followers_count, following_count, and statuses_count from the /api/v1/accounts/verify_credentials endpoint.
   * Bluesky implementation: Fetch followersCount, followsCount, and postsCount from the app.bsky.actor.getProfile XRPC method.

  2. Database Schema Expansion
  Create tables to store account-level snapshots, mirroring the existing post metrics structure.
   * account_metrics: Stores the latest "current" values for an account.
   * account_metrics_history: Stores daily snapshots (recorded_at date) to allow for historical growth charts (e.g., follower growth over 30 days).

  3. Scheduler Enhancements
   * AccountMetricsJob: Add a new periodic job in internal/scheduler/scheduler.go that runs once every 24 hours to capture account snapshots.
   * Immediate Sync: Modify the account connection flow (in API/Store) to trigger an initial metrics sync immediately after a user connects a new account, ensuring the
     dashboard isn't empty for the first 24 hours.

  4. API & Visualization
   * Endpoints: Update api/analytics.go to include GET /v1/teams/{teamID}/analytics/account/{accountID}/growth which returns time-series data from
     account_metrics_history.
   * Frontend: Add "Follower Growth" and "Total Reach" line charts to the AnalyticsView.tsx using recharts, allowing users to filter by specific accounts or view an
     aggregate for the whole team.

  5. Data Retention & Cleanup (Optional)
  goloom should implement a configurable retention policy (e.g., "Keep history for 365 days") in the config to manage
  database growth for very large installations.
