# Social Media Integration: Technical Data Exchange

This document defines the data structures and communication patterns between the application and social media providers.

## 1. Authentication & Credentials
For every connected account, the system stores:
*   **Access Token:** Long-lived or short-lived Bearer token.
*   **Refresh Token:** Used to obtain new access tokens without user intervention.
*   **Expiration Timestamp:** To trigger background refresh jobs.
*   **Provider Metadata:** Unique Platform ID, Profile URL, and Avatar URL.

## 2. Publishing Workflow (Request Payload)
When sending a post to a provider (e.g., Mastodon), the payload includes:
*   **Content:** The raw text (UTF-8).
*   **Media IDs:** An array of IDs for media already uploaded to the platform's CDN (Social platforms usually require a two-step process: Upload Media -> Get ID -> Attach ID to Post).
*   **Visibility/Privacy:** String constant (e.g., `public`, `unlisted`, `private`).
*   **Scheduled Time:** (Optional) If using the platform's native scheduling rather than the application's internal scheduler.

## 3. Media Upload (Request Payload)
*   **Binary Data:** The raw file stream.
*   **Filename & Mime-Type:** Used for platform-side validation.
*   **Alt Text:** Accessibility descriptions for images/videos.

## 4. Provider Response Structure
After a successful post, the provider returns:
*   **Remote ID:** The unique identifier of the post on the social network.
*   **Permalinks:** The public URL to view the post.
*   **Platform-specific Data:** (e.g., Mastodon's `uri`, X's `edit_history_tweet_ids`).
