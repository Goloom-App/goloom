package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
)

func (a *API) handleListReviewQueue(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.ListAutomationReviewDrafts(r.Context(), r.PathValue("teamID"), 200)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": sliceOrEmpty(items)})
}

type adminSeedAutomationDraftRequest struct {
	TeamID         string    `json:"team_id"`
	Title          string    `json:"title"`
	Content        string    `json:"content"`
	TargetAccounts []string  `json:"target_accounts"`
	ScheduledAt    time.Time `json:"scheduled_at"`
}

// handleAdminSeedAutomationDraft creates an automation-source draft (admin / E2E only).
func (a *API) handleAdminSeedAutomationDraft(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if !principal.User.IsAdmin {
		a.writeError(w, r, "forbidden", http.StatusForbidden)
		return
	}
	var input adminSeedAutomationDraftRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	if input.ScheduledAt.IsZero() {
		input.ScheduledAt = time.Now().UTC().Add(-2 * time.Hour)
	}
	postInput := domain.CreatePostInput{
		Title:          input.Title,
		Content:        input.Content,
		ScheduledAt:    input.ScheduledAt,
		TargetAccounts: input.TargetAccounts,
		Draft:          true,
		Source:         domain.PostSourceAutomation,
	}
	postInput.EnsureTitle()
	post, err := a.store.CreateScheduledPost(r.Context(), input.TeamID, principal, postInput)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.notifyReviewCreated(r.Context(), input.TeamID, post)
	auth.WriteJSON(w, http.StatusCreated, post)
}

type adminSeedE2EAccountRequest struct {
	TeamID string `json:"team_id"`
}

// handleAdminSeedE2EAccount creates a fake connected account on the team so
// queue/composer E2E specs can save a post targeting a real account id, without
// a network call to the provider (admin / E2E only).
func (a *API) handleAdminSeedE2EAccount(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if !principal.User.IsAdmin {
		a.writeError(w, r, "forbidden", http.StatusForbidden)
		return
	}
	var input adminSeedE2EAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	if input.TeamID == "" {
		a.writeError(w, r, "team_id_required", http.StatusBadRequest)
		return
	}
	account, err := a.store.CreateAccount(r.Context(), input.TeamID, domain.ConnectedAccount{
		Provider:     "mastodon",
		AuthType:     domain.AccountAuthTypeOAuthToken,
		InstanceURL:  "https://e2e.invalid",
		Username:     fmt.Sprintf("e2e-%d", time.Now().UnixNano()),
		AccessToken:  "e2e-token",
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusCreated, account)
}
