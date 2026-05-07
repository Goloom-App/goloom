# Analytics: Parameters & Metrics Collection

This document outlines how performance data is gathered, processed, and utilized.

## 1. Metrics Collection Logic
The system uses a scheduled background job (`FetchMetricsJob`) that iterates through all active social accounts.

*   **Frequency:** Typically every 6-24 hours depending on post age.
*   **Mechanism:** The job calls the `getMetrics($postId)` method for each post published within the last 30 days.

## 2. Collected Parameters (by Platform)

### Mastodon / ActivityPub
*   **`reblogs_count`:** Number of boosts/shares.
*   **`favourites_count`:** Number of likes/stars.
*   **`replies_count`:** Number of direct responses.

### X (Twitter)
*   **`impression_count`:** Number of times the tweet was seen.
*   **`like_count`:** Number of favorites.
*   **`retweet_count`:** Number of shares.
*   **`reply_count`:** Number of comments.

### Facebook / Instagram
*   **`reach`:** Unique users who saw the post.
*   **`impressions`:** Total views.
*   **`engagement`:** Sum of likes, comments, and shares.

## 3. Data Storage & Usage
*   **`mixpost_metrics` Table:** Stores snapshots of metrics with a timestamp.
    *   `account_id`, `post_id`, `parameter_name` (e.g., 'likes'), `value`, `date`.
*   **Historical Tracking:** By storing daily snapshots, the system can calculate delta values (e.g., "Gained 50 likes in the last 24 hours").
*   **Usage in UI:**
    *   **Aggregation:** Summing metrics across all posts in a workspace to show "Total Engagement".
    *   **Reporting:** Generating CSV/PDF reports for clients based on these parameters.
    *   **Optimization:** Comparing metrics against "Time of Posting" to suggest future schedule improvements.
