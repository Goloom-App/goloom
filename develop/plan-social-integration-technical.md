# Plan: Social Media Integration Technical Implementation

This plan outlines the changes required to align the current codebase with the technical specifications in `docs/social-integration-technical.md`.

## 1. Authentication & Credentials Enhancements

### Domain Model Changes (`internal/domain/models.go`)
- Update `SocialAccount` to include `AccessTokenExpiresAt`.
- Update `ConnectedAccount` to include `AccessTokenExpiresAt`.

### Database Schema Updates (`db/schema.sql`)
- Add `access_token_expires_at` (timestamptz) to `social_accounts`.

### Backend Logic (`internal/store/`)
- Ensure `AccessTokenExpiresAt` is persisted and retrieved.
- Implement token refresh logic in the provider layer or a dedicated service.

## 2. Publishing Workflow Improvements

### Domain Model Changes (`internal/domain/models.go`)
- Update `ScheduledPost` to support visibility/privacy.
- (Optional) Support platform-specific metadata in `ScheduledPostTarget`.

### Provider Interface Changes (`internal/provider/provider.go`)
- Update `PublishRequest` to include:
  ```go
  type PublishRequest struct {
      Content     string
      MediaIDs    []string
      Visibility  string // public, unlisted, private, direct
      ScheduledAt *time.Time
  }
  ```
- Add `UploadMedia` method to `SocialMediaProvider`:
  ```go
  UploadMedia(ctx context.Context, account domain.SocialAccount, auth PublishAuth, file io.Reader, filename, mimeType, altText string) (string, error)
  ```

### Implementation (e.g., `internal/provider/mastodon.go`)
- Implement `UploadMedia` using Mastodon's `/api/v2/media` endpoint.
- Update `Publish` to use the new `PublishRequest` fields and attach media IDs.

## 3. Provider Response Structure

### Provider Interface Changes (`internal/provider/provider.go`)
- Expand `PublishResult` to include platform-specific metadata:
  ```go
  type PublishResult struct {
      RemoteID string
      URL      string
      Metadata map[string]string // e.g., "uri" for Mastodon
  }
  ```

### Backend Logic (`internal/scheduler/scheduler.go`)
- Ensure that the full `PublishResult` (including metadata) is captured and potentially stored in `scheduled_post_targets`.

## 4. Media Upload Logic

### API Layer (`api/http.go`)
- Create an endpoint `POST /v1/teams/{teamID}/media/upload` that:
  1. Accepts multi-part form data.
  2. Identifies the target account(s).
  3. Calls the provider's `UploadMedia` method.
  4. Returns the platform-specific Media ID.

### Frontend Integration
- Update the post composer to allow media attachments.
- Handle the asynchronous upload of media before post submission.

## 5. Implementation Phases

### Phase 1: Authentication & Schema
- Update `domain.SocialAccount` and `db/schema.sql`.
- Update repository/store logic for the new column.
- Update OAuth callback logic to capture `expires_in` if provided by the platform.

### Phase 2: Provider Interface & Mastodon Implementation
- Update `SocialMediaProvider` interface.
- Implement `UploadMedia` in `MastodonProvider`.
- Update `Publish` in `MastodonProvider`.

### Phase 3: API & Scheduler Integration
- Update the scheduler to pass visibility and media IDs to the provider.
- Implement the media upload API endpoint.

### Phase 4: Frontend (Optional/Follow-up)
- UI changes to support visibility selection and media attachments.

## 6. Validation
- Unit tests for token expiration logic.
- Mock-based tests for the two-step publishing process (Upload -> Publish).
- Integration test with a live Mastodon instance (or mock server) verifying visibility settings.
