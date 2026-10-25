package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/socialtokens"
)

type threadAuthorJSON struct {
	Handle      string `json:"handle"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
	ProfileURL  string `json:"profile_url"`
}

type threadReplyJSON struct {
	RemoteID    string           `json:"remote_id"`
	ParentID    string           `json:"parent_id"`
	URL         string           `json:"url"`
	Content     string           `json:"content"`
	PublishedAt string           `json:"published_at"`
	Author      threadAuthorJSON `json:"author"`
}

// handlePostThread lists replies below one published target of a post (read-only, fetched live).
func (a *API) handlePostThread(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	teamID := r.PathValue("teamID")
	postID := r.PathValue("postID")
	accountID := r.PathValue("accountID")

	if _, err := a.store.GetScheduledPost(ctx, teamID, postID); err != nil {
		a.writeError(w, r, "not_found", http.StatusNotFound)
		return
	}
	target, err := a.store.GetPublishedTarget(ctx, postID, accountID)
	if errors.Is(err, sql.ErrNoRows) {
		a.writeError(w, r, "not_found", http.StatusNotFound)
		return
	}
	if err != nil {
		a.writeError(w, r, "internal_error", http.StatusInternalServerError)
		return
	}
	publishedURL := strings.TrimSpace(target.PublishedURL)
	if publishedURL == "" && strings.TrimSpace(target.RemotePostID) == "" {
		a.writeError(w, r, "post_not_published", http.StatusConflict)
		return
	}
	accounts, err := a.store.GetAccountsByIDs(ctx, teamID, []string{accountID})
	if err != nil || len(accounts) == 0 {
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

	logCtx := []any{"post_id", postID, "account_id", accountID, "provider", account.Provider}
	if fresh, err := socialtokens.EnsureMastodonFresh(ctx, a.store, a.providers, account); err == nil {
		account = fresh
	}
	token, err := a.store.DecryptAccessToken(account)
	if err != nil {
		a.warnThread(ctx, "thread fetch decrypt access failed", append(logCtx, "error", err)...)
		a.writeError(w, r, "thread_fetch_failed", http.StatusBadGateway)
		return
	}
	refreshToken, err := a.store.DecryptRefreshToken(account)
	if err != nil {
		a.warnThread(ctx, "thread fetch decrypt refresh failed", append(logCtx, "error", err)...)
		a.writeError(w, r, "thread_fetch_failed", http.StatusBadGateway)
		return
	}
	replies, err := fetcher.GetThread(a.providerContext(ctx), account, provider.PublishAuth{
		AccessToken:  token,
		RefreshToken: refreshToken,
	}, provider.ThreadTarget{PublishedURL: publishedURL, RemoteID: target.RemotePostID, Metadata: target.PublishMetadata})
	if err != nil {
		a.warnThread(ctx, "thread fetch failed", append(logCtx, "published_url", publishedURL, "error", err)...)
		a.writeError(w, r, "thread_fetch_failed", http.StatusBadGateway)
		return
	}

	out := make([]threadReplyJSON, 0, len(replies))
	for _, rp := range replies {
		published := ""
		if !rp.PublishedAt.IsZero() {
			published = rp.PublishedAt.UTC().Format(time.RFC3339)
		}
		out = append(out, threadReplyJSON{
			RemoteID: rp.RemoteID, ParentID: rp.ParentID, URL: rp.URL, Content: rp.Content, PublishedAt: published,
			Author: threadAuthorJSON{Handle: rp.Author.Handle, DisplayName: rp.Author.DisplayName, AvatarURL: rp.Author.AvatarURL, ProfileURL: rp.Author.ProfileURL},
		})
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{
		"post_id":       postID,
		"account_id":    accountID,
		"provider":      account.Provider,
		"published_url": publishedURL,
		"fetched_at":    time.Now().UTC().Format(time.RFC3339),
		"replies":       out,
	})
}

func (a *API) warnThread(ctx context.Context, msg string, args ...any) {
	if a.log != nil {
		a.log.WarnContext(ctx, msg, args...)
	}
}
