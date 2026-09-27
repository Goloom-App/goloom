package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/push"
)

// PushSender exposes the notifier so internal/app can wire it into the
// scheduler and other job runners.
func (a *API) PushSender() *push.Sender {
	return a.push
}

// SetPushSender replaces the notifier (used by tests to inject a sender whose
// HTTP client is pinned to a test endpoint).
func (a *API) SetPushSender(sender *push.Sender) {
	a.push = sender
}

type pushSubscriptionKeys struct {
	P256dh string `json:"p256dh"`
	Auth   string `json:"auth"`
}

type createPushSubscriptionRequest struct {
	Endpoint string               `json:"endpoint"`
	Keys     pushSubscriptionKeys `json:"keys"`
}

// notifyReviewCreated pushes a "new review" notification when a created post
// is a review item (draft automation post). Push failures never propagate to
// the caller — review creation must not fail because a notification could not
// be sent.
func (a *API) notifyReviewCreated(ctx context.Context, teamID string, post domain.ScheduledPost) {
	if a.push == nil || post.Status != domain.PostStatusDraft || post.Source != domain.PostSourceAutomation {
		return
	}
	team, err := a.store.GetTeamByID(ctx, teamID)
	if err != nil {
		a.log.Warn("push: team lookup for review notification", "team_id", teamID, "error", err)
		return
	}
	a.push.SendNewReview(ctx, team, post)
}

func (a *API) handleGetMyVAPIDPublicKey(w http.ResponseWriter, r *http.Request) {
	if a.push == nil {
		http.Error(w, "push notifications unavailable", http.StatusServiceUnavailable)
		return
	}
	pub, err := a.push.VAPIDPublicKey(r.Context())
	if err != nil {
		a.log.Error("vapid public key", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]string{"public_key": pub})
}

func (a *API) handleListMyPushSubscriptions(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	subs, err := a.store.ListPushSubscriptions(r.Context(), principal.User.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": sliceOrEmpty(subs)})
}

func (a *API) handleCreateMyPushSubscription(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	var input createPushSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	trimmedEndpoint := strings.TrimSpace(input.Endpoint)
	// Only https endpoints are accepted: an http push endpoint would let any
	// authenticated user make the server POST VAPID-signed payloads to an
	// arbitrary URL (SSRF through test delivery).
	if !strings.HasPrefix(trimmedEndpoint, "https://") {
		a.writeError(w, r, "push_subscription_endpoint_https_required", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(input.Keys.P256dh) == "" || strings.TrimSpace(input.Keys.Auth) == "" {
		a.writeError(w, r, "push_subscription_keys_required", http.StatusBadRequest)
		return
	}
	sub, err := a.store.CreatePushSubscription(r.Context(), principal.User.ID, domain.PushSubscription{
		UserID:   principal.User.ID,
		Endpoint: trimmedEndpoint,
		P256dh:   strings.TrimSpace(input.Keys.P256dh),
		Auth:     strings.TrimSpace(input.Keys.Auth),
		Enabled:  true,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusCreated, map[string]any{"item": sub})
}

func (a *API) handleUpdateMyPushSubscription(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	var input struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	sub, err := a.store.UpdatePushSubscriptionEnabled(r.Context(), principal.User.ID, r.PathValue("subID"), input.Enabled)
	if err != nil {
		if errors.Is(err, domain.ErrPushSubscriptionNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"item": sub})
}

func (a *API) handleDeleteMyPushSubscription(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if err := a.store.DeletePushSubscription(r.Context(), principal.User.ID, r.PathValue("subID")); err != nil {
		if errors.Is(err, domain.ErrPushSubscriptionNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleSendMyPushTest(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if a.push == nil {
		http.Error(w, "push notifications unavailable", http.StatusServiceUnavailable)
		return
	}
	sub, err := a.store.GetPushSubscription(r.Context(), principal.User.ID, r.PathValue("subID"))
	if err != nil {
		if errors.Is(err, domain.ErrPushSubscriptionNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := a.push.SendTest(r.Context(), principal.User.ID, sub); err != nil {
		a.log.Warn("push test failed", "subscription_id", sub.ID, "error", err)
		a.writeError(w, r, "push_test_failed", http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleListMyTeamNotificationPrefs(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	// Only owner/editor members receive review notifications; viewers are not
	// listed at all. A missing pref row means "enabled" (see store default),
	// so every qualifying team gets an explicit (default-enabled) item here.
	teams, err := a.store.ListTeamsForUser(r.Context(), principal.User.ID, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rows, err := a.store.ListTeamNotificationPrefs(r.Context(), principal.User.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	overrides := make(map[string]bool, len(rows))
	for _, p := range rows {
		overrides[p.TeamID] = p.Enabled
	}
	var items []domain.TeamNotificationPref
	for _, tm := range teams {
		allowed, err := a.store.UserHasAnyTeamRole(r.Context(), principal.User.ID, tm.ID, domain.RoleOwner, domain.RoleEditor)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !allowed {
			continue
		}
		enabled := true
		if v, ok := overrides[tm.ID]; ok {
			enabled = v
		}
		items = append(items, domain.TeamNotificationPref{
			UserID:   principal.User.ID,
			TeamID:   tm.ID,
			TeamName: tm.Name,
			Enabled:  enabled,
		})
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": sliceOrEmpty(items)})
}

func (a *API) handleSetMyTeamNotificationPref(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	teamID := r.PathValue("teamID")
	// Only owner/editor members may toggle review notifications for a team —
	// viewers never receive them, so they must not be able to configure them.
	allowed, err := a.store.UserHasAnyTeamRole(r.Context(), principal.User.ID, teamID, domain.RoleOwner, domain.RoleEditor)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !allowed {
		http.Error(w, "not a team owner or editor", http.StatusForbidden)
		return
	}
	var input struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	pref, err := a.store.SetTeamNotificationPref(r.Context(), principal.User.ID, teamID, input.Enabled)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Surface the team name so the UI never has to render the raw ID.
	if team, err := a.store.GetTeamByID(r.Context(), teamID); err == nil {
		pref.TeamName = team.Name
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"item": pref})
}

func (a *API) handleMyReviewCounts(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	teams, err := a.store.ListTeamsForUser(r.Context(), principal.User.ID, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	byTeam := make(map[string]int, len(teams))
	total := 0
	for _, team := range teams {
		// Only owner/editor members receive review notifications and get
		// delivery eligibility; a viewer membership must not inflate counts.
		allowed, err := a.store.UserHasAnyTeamRole(r.Context(), principal.User.ID, team.ID, domain.RoleOwner, domain.RoleEditor)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !allowed {
			continue
		}
		count, err := a.store.CountOpenReviewItems(r.Context(), team.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		byTeam[team.ID] = count
		total += count
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"total": total, "by_team": byTeam})
}