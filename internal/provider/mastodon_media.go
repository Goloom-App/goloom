package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

// mastodonCompatibleStatusPayload builds the JSON body for Mastodon-compatible /api/v1/statuses posts.
func mastodonCompatibleStatusPayload(req PublishRequest) map[string]any {
	payload := map[string]any{"status": req.Content}
	ids := domain.NormalizeMediaIDs(req.MediaIDs)
	if len(ids) > 0 {
		payload["media_ids"] = ids
	}
	if vis := domain.NormalizePostVisibility(req.Visibility); vis != "" {
		payload["visibility"] = vis
	}
	if req.ScheduledAt != nil && !req.ScheduledAt.IsZero() {
		payload["scheduled_at"] = req.ScheduledAt.UTC().Format(time.RFC3339Nano)
	}
	if sp := strings.TrimSpace(req.SpoilerText); sp != "" {
		payload["spoiler_text"] = sp
	}
	if req.Sensitive {
		payload["sensitive"] = true
	}
	return payload
}

type mastodonMediaAttachmentState struct {
	ID    string  `json:"id"`
	URL   *string `json:"url"`
	Error *string `json:"error"`
}

const (
	mastodonMediaPollInterval   = 500 * time.Millisecond
	mastodonMediaWaitDeadline   = 90 * time.Second
	mastodonMediaErrorBodyLimit = 2048
)

func waitMastodonMediaReady(ctx context.Context, apiBase, accessToken, mediaID string) error {
	deadline := time.Now().Add(mastodonMediaWaitDeadline)
	base := strings.TrimRight(apiBase, "/")
	escaped := url.PathEscape(strings.TrimSpace(mediaID))
	endpoint := base + "/api/v1/media/" + escaped

	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return fmt.Errorf("build media poll request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
		resp, err := defaultHTTPClient.Do(req)
		if err != nil {
			return fmt.Errorf("mastodon media poll: %w", err)
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, mastodonMediaErrorBodyLimit))
		_ = resp.Body.Close()
		if resp.StatusCode >= http.StatusBadRequest {
			return fmt.Errorf("mastodon media poll failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		var st mastodonMediaAttachmentState
		if err := json.Unmarshal(body, &st); err != nil {
			return fmt.Errorf("decode mastodon media poll: %w", err)
		}
		if st.Error != nil && strings.TrimSpace(*st.Error) != "" {
			return fmt.Errorf("mastodon media processing failed: %s", strings.TrimSpace(*st.Error))
		}
		if st.URL != nil && strings.TrimSpace(*st.URL) != "" {
			return nil
		}
		select {
		case <-time.After(mastodonMediaPollInterval):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return errors.New("timeout waiting for mastodon media to finish processing")
}

func uploadMastodonV2Media(ctx context.Context, apiBase, accessToken string, file io.Reader, filename, mimeType, altText string) (string, error) {
	_ = mimeType // reserved for future Content-Type hints on the file part
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("description", altText); err != nil {
		return "", err
	}
	fn := strings.TrimSpace(filename)
	if fn == "" {
		fn = "upload"
	}
	fw, err := mw.CreateFormFile("file", fn)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(fw, file); err != nil {
		return "", err
	}
	if err := mw.Close(); err != nil {
		return "", err
	}

	endpoint := strings.TrimRight(apiBase, "/") + "/api/v2/media"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &buf)
	if err != nil {
		return "", fmt.Errorf("build media upload: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("mastodon media upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, mastodonMediaErrorBodyLimit))
		msg := strings.TrimSpace(string(errBody))
		if msg == "" {
			return "", fmt.Errorf("mastodon media upload failed with status %d", resp.StatusCode)
		}
		return "", fmt.Errorf("mastodon media upload failed with status %d: %s", resp.StatusCode, msg)
	}

	var out mastodonMediaAttachmentState
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode mastodon media response: %w", err)
	}
	if out.Error != nil && strings.TrimSpace(*out.Error) != "" {
		return "", fmt.Errorf("mastodon media upload failed: %s", strings.TrimSpace(*out.Error))
	}
	id := strings.TrimSpace(out.ID)
	if id == "" {
		return "", errors.New("mastodon media upload returned no id")
	}
	if out.URL != nil && strings.TrimSpace(*out.URL) != "" {
		return id, nil
	}
	if err := waitMastodonMediaReady(ctx, apiBase, accessToken, id); err != nil {
		return "", err
	}
	return id, nil
}
