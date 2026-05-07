# Plan: Media Handling Infrastructure

This document outlines the architectural changes required to support a robust, persistent, and multi-tenant media library in Goloom, while streamlining the social media publishing workflow.

## 1. Current State & Limitations
*   **Direct-to-Provider Upload:** Media is currently uploaded directly to a social provider (e.g., Mastodon) before being attached to a post.
*   **No Persistence:** Once uploaded, the local "knowledge" of the media (the remote ID) is only stored in `localStorage` or transient session state.
*   **No Cross-Platform Reuse:** Media uploaded for Mastodon cannot be easily reused for Bluesky without re-uploading the original file.
*   **Missing Previews:** Without a local copy, the UI cannot reliably show previews of media once the session is cleared.

## 2. Proposed Architecture: Two-Stage Media Flow

We will transition to a local-first media management strategy.

### 2.1 Stage 1: Local Ingest (Goloom Store)
1.  **Upload:** User uploads a file to the Goloom backend.
2.  **Storage:** The file is stored in a workspace-scoped directory: `data/media/{team_id}/{file_hash}`.
3.  **Database:** Metadata is recorded in a new `media_library` table.
4.  **Deduplication:** Content-addressable storage (SHA256) ensures the same file isn't stored multiple times for the same team.

### 2.2 Stage 2: Platform Synchronization (Just-in-Time)
1.  **Drafting:** User selects a file from the Media Library for a post.
2.  **Publishing:** When the scheduler starts processing a post:
    *   It checks if the file has already been uploaded to the target account's provider.
    *   If not, it performs the provider upload using the local file.
    *   It maps the local media ID to the provider-specific remote ID.
3.  **Caching:** Remote IDs are cached in a `media_provider_mappings` table to avoid redundant uploads for future posts to the same account.

## 3. Database Schema Changes

### 3.1 `media_items` (The Library)
Stores the "source of truth" for a piece of media within a team.
*   `id` (UUID): Primary key.
*   `team_id` (UUID): Ownership.
*   `sha256` (Text): Content hash.
*   `filename` (Text): Original name.
*   `mime_type` (Text): File type.
*   `size_bytes` (BigInt): File size.
*   `width`/`height` (Int): Dimensions (optional metadata).
*   `created_at` (Timestamptz).

### 3.2 `media_provider_mappings` (The Sync Cache)
Links local media to remote IDs on specific social accounts.
*   `media_id` (UUID): Reference to `media_items`.
*   `account_id` (UUID): Reference to `social_accounts`.
*   `remote_id` (Text): The ID returned by the provider (e.g. Mastodon attachment ID).
*   `expires_at` (Timestamptz): Optional, for providers where media IDs are transient.

## 4. Backend Implementation

### 4.1 Storage Strategy
*   **Local Filesystem:** Default storage under `data/media/`.
*   **Cleanup Service:** A background job to remove "orphaned" media files (files on disk not referenced in the DB) or old files not attached to any post.

### 4.2 API Endpoints
*   `GET /v1/teams/{teamID}/media`: List library items.
*   `POST /v1/teams/{teamID}/media`: Upload new media to the library.
*   `DELETE /v1/teams/{teamID}/media/{mediaID}`: Remove from library (and disk).
*   `GET /v1/teams/{teamID}/media/{mediaID}/preview`: Serve the local file with proper caching headers.

## 5. Frontend & UI Evolution

### 5.1 Media Library View
*   **Grid Gallery:** Display true previews of all uploaded media using the backend preview endpoint.
*   **Search/Filter:** Filter by file type or date.
*   **Metadata Editing:** Allow adding Alt Text directly to the library item (used as default for all posts).

### 5.2 Composer Integration
*   **"Pick from Library"**: Instead of just "Choose File", users can browse their team's existing media.
*   **Drag & Drop:** Drop files directly into the composer to both upload to the library and attach to the post in one go.

## 6. Implementation Steps

1.  **Step 1: Database Migration.** Add `media_items` and `media_provider_mappings`.
2.  **Step 2: Backend Upload Logic.** Implement hash-based storage and metadata recording.
3.  **Step 3: Asset Serving.** Create a secure endpoint to serve media files for UI previews.
4.  **Step 4: Scheduler Update.** Modify the publishing logic to use local files for provider uploads just-in-time.
5.  **Step 5: Frontend Refactor.** Replace the current `localStorage`-based media view with the new API-backed gallery.
6.  **Step 6: Cleanup.** Remove legacy "direct upload" logic and clean up unused provider code (Giphy/Unsplash references).

---

## Implementation status (verified in repo)

Steps 1–5 are implemented: `media_items` + `media_provider_mappings`, hash-based filesystem storage under `data/media/{team_id}/{sha256}`, REST list/upload/delete/preview, scheduler JIT upload via `syncMediaToProvider` with mapping cache. The composer uses the library upload (`POST /v1/teams/{team}/media`). Legacy `POST .../media/upload` was removed.

**Gaps vs this document**

- DB table name is **`media_items`** (not `media_library`).  
- Alt text defaults in library, search/filter gallery, media cleanup jobs, Giphy/Unsplash pruning, and Postgres/SQLite **online** migration for deployments that already have `media_items` without the unique `(team_id, sha256)` index are not fully covered—new installs get `ux_media_items_team_sha256` from `schema.sql`.
