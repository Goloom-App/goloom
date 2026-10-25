package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
)

type threadAuthorJSON struct {
	Handle      string `json:"handle"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
	ProfileURL  string `json:"profile_url"`
}

type threadReplyJSON struct {
	RemoteID       string           `json:"remote_id"`
	ParentID       string           `json:"parent_id"`
	URL            string           `json:"url"`
	Content        string           `json:"content"`
	PublishedAt    string           `json:"published_at"`
	ContentWarning string           `json:"content_warning,omitempty"`
	Sensitive      bool             `json:"sensitive"`
	Author         threadAuthorJSON `json:"author"`
}

// handlePostThread lists replies below one published target of a post (read-only, fetched live).
func (a *API) handlePostThread(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	teamID := r.PathValue("teamID")
	postID := r.PathValue("postID")
	accountID := r.PathValue("accountID")

	notFound := func(err error) bool {
		return errors.Is(err, domain.ErrNotFound) || errors.Is(err, sql.ErrNoRows)
	}
	logCtx := []any{"post_id", postID, "account_id", accountID}
	internal := func(msg string, err error) {
		a.warnThread(ctx, msg, append(logCtx, "error", err)...)
		a.writeError(w, r, "internal_error", http.StatusInternalServerError)
	}

	if _, err := a.store.GetScheduledPost(ctx, teamID, postID); err != nil {
		if notFound(err) {
			a.writeError(w, r, "not_found", http.StatusNotFound)
		} else {
			internal("thread: load post failed", err)
		}
		return
	}
	target, err := a.store.GetPublishedTarget(ctx, postID, accountID)
	if err != nil {
		if notFound(err) {
			a.writeError(w, r, "not_found", http.StatusNotFound)
		} else {
			internal("thread: load target failed", err)
		}
		return
	}
	publishedURL := strings.TrimSpace(target.PublishedURL)
	if publishedURL == "" && strings.TrimSpace(target.RemotePostID) == "" {
		a.writeError(w, r, "post_not_published", http.StatusConflict)
		return
	}
	accounts, err := a.store.GetAccountsByIDs(ctx, teamID, []string{accountID})
	if err != nil {
		internal("thread: load account failed", err)
		return
	}
	if len(accounts) == 0 {
		a.writeError(w, r, "not_found", http.StatusNotFound)
		return
	}
	account := accounts[0]
	impl, ok := a.providers.Get(account.Provider)
	if !ok {
		a.writeError(w, r, "thread_unsupported", http.StatusUnprocessableEntity)
		return
	}
	fetcher, ok := impl.(provider.ThreadFetcher)
	if !ok {
		a.writeError(w, r, "thread_unsupported", http.StatusUnprocessableEntity)
		return
	}

	logCtx = append(logCtx, "provider", account.Provider)
	// Read path: no token refresh (it writes and races refresh-token rotation) and no refresh token.
	token, err := a.store.DecryptAccessToken(account)
	if err != nil {
		a.warnThread(ctx, "thread fetch decrypt access failed", append(logCtx, "error", err)...)
		a.writeError(w, r, "thread_fetch_failed", http.StatusBadGateway)
		return
	}
	result, fetchedAt, err := a.threads.get(a.providerContext(ctx), teamID+"|"+postID+"|"+accountID, func(fctx context.Context) (provider.ThreadResult, error) {
		return fetcher.GetThread(fctx, account, provider.PublishAuth{AccessToken: token},
			provider.ThreadTarget{PublishedURL: publishedURL, RemoteID: target.RemotePostID, Metadata: target.PublishMetadata})
	})
	if errors.Is(err, provider.ErrThreadTargetUnresolvable) {
		a.writeError(w, r, "thread_target_unresolvable", http.StatusConflict)
		return
	}
	if err != nil {
		a.warnThread(ctx, "thread fetch failed", append(logCtx, "published_url", publishedURL, "error", err)...)
		a.writeError(w, r, "thread_fetch_failed", http.StatusBadGateway)
		return
	}

	out := make([]threadReplyJSON, 0, len(result.Replies))
	for _, rp := range result.Replies {
		published := ""
		if !rp.PublishedAt.IsZero() {
			published = rp.PublishedAt.UTC().Format(time.RFC3339)
		}
		out = append(out, threadReplyJSON{
			RemoteID: rp.RemoteID, ParentID: rp.ParentID, URL: rp.URL, Content: rp.Content, PublishedAt: published,
			ContentWarning: rp.ContentWarning, Sensitive: rp.Sensitive,
			Author: threadAuthorJSON{Handle: rp.Author.Handle, DisplayName: rp.Author.DisplayName, AvatarURL: rp.Author.AvatarURL, ProfileURL: rp.Author.ProfileURL},
		})
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{
		"post_id":       postID,
		"account_id":    accountID,
		"provider":      account.Provider,
		"published_url": publishedURL,
		"fetched_at":    fetchedAt.Format(time.RFC3339),
		"truncated":     result.Truncated,
		"replies":       out,
	})
}

func (a *API) warnThread(ctx context.Context, msg string, args ...any) {
	if a.log != nil {
		a.log.WarnContext(ctx, msg, args...)
	}
}
