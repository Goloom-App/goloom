package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
)

type createAIDraftRequest struct {
	Title                  string            `json:"title"`
	Content                string            `json:"content"`
	AccountIDs             []string          `json:"account_ids"`
	AccountContentOverride map[string]string `json:"account_content_override"`
	ScheduledAt            *time.Time        `json:"scheduled_at"`
	Schedule               bool              `json:"schedule"`
	AIJobID                string            `json:"ai_job_id"`
	Metadata               map[string]any    `json:"metadata"`
}

func (a *API) handleCreateAIDraft(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		http.Error(w, "missing principal", http.StatusUnauthorized)
		return
	}

	var input createAIDraftRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}

	content := strings.TrimSpace(input.Content)
	if content == "" {
		a.writeError(w, r, "content_required", http.StatusBadRequest)
		return
	}

	teamID := r.PathValue("teamID")
	targets := domain.NormalizeMediaIDs(input.AccountIDs)

	// Validate target accounts belong to the team (and that overrides reference a
	// target) before persisting. This endpoint is user-driven, so it must not be
	// able to target another team's accounts.
	var overrideKeys []string
	for id, c := range input.AccountContentOverride {
		if strings.TrimSpace(c) != "" {
			overrideKeys = append(overrideKeys, id)
		}
	}
	if _, _, err := a.posts.ResolveTargets(r.Context(), teamID, targets, overrideKeys); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	postInput := domain.CreatePostInput{
		Title:                  input.Title,
		Content:                content,
		TargetAccounts:         targets,
		AccountContentOverride: input.AccountContentOverride,
		ScheduledAt:            time.Now().UTC(),
		Draft:                  true,
		AuthorUserID:           &principal.User.ID,
	}
	postInput.Normalize()
	postInput.EnsureTitle()
	if input.ScheduledAt != nil && !input.ScheduledAt.IsZero() {
		postInput.ScheduledAt = input.ScheduledAt.UTC()
	}
	if input.Schedule {
		postInput.Draft = false
	}

	profile, err := a.store.GetTeamProfile(r.Context(), teamID)
	if err == nil && profile.AutoPublishEnabled && input.ScheduledAt != nil && !input.ScheduledAt.IsZero() {
		postInput.Draft = false
	}

	post, err := a.store.CreateScheduledPost(r.Context(), teamID, *principal, postInput)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	auth.WriteJSON(w, http.StatusCreated, post)
}
