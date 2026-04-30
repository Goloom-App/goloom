package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/store/postgres"
	"github.com/microcosm-cc/bluemonday"
)

type API struct {
	store     *postgres.Store
	auth      *auth.Service
	providers *provider.Registry
	sanitizer *bluemonday.Policy
}

type validationResponse struct {
	MaxChars      int               `json:"max_chars"`
	ContentLength int               `json:"content_length"`
	Valid         bool              `json:"valid"`
	Destinations  []destinationInfo `json:"destinations"`
}

type destinationInfo struct {
	AccountID string `json:"account_id"`
	Provider  string `json:"provider"`
	MaxChars  int    `json:"max_chars"`
}

func New(store *postgres.Store, authService *auth.Service, providers *provider.Registry) *API {
	return &API{
		store:     store,
		auth:      authService,
		providers: providers,
		sanitizer: bluemonday.UGCPolicy(),
	}
}

func (a *API) Handler(limiter *security.Limiter, allowedOrigins []string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.handleHealth)
	mux.HandleFunc("GET /v1/providers", a.handleProviders)

	mux.Handle("GET /v1/me", a.auth.RequireAuth(http.HandlerFunc(a.handleMe)))
	mux.Handle("GET /v1/teams/{teamID}/accounts", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleListAccounts))))
	mux.Handle("POST /v1/teams/{teamID}/accounts", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleCreateAccount))))
	mux.Handle("GET /v1/teams/{teamID}/posts", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleListPosts))))
	mux.Handle("POST /v1/teams/{teamID}/posts", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleCreatePost))))
	mux.Handle("POST /v1/teams/{teamID}/posts/validate", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleValidatePost))))
	mux.Handle("GET /v1/teams/{teamID}/posts/{postID}", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleGetPost))))
	mux.Handle("PATCH /v1/teams/{teamID}/posts/{postID}", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleUpdatePost))))
	mux.Handle("DELETE /v1/teams/{teamID}/posts/{postID}", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleDeletePost))))
	mux.Handle("POST /v1/teams/{teamID}/posts/{postID}/cancel", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleCancelPost))))

	return security.CORSMiddleware(allowedOrigins)(limiter.Middleware(mux))
}

func (a *API) handleHealth(w http.ResponseWriter, _ *http.Request) {
	auth.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) handleProviders(w http.ResponseWriter, _ *http.Request) {
	auth.WriteJSON(w, http.StatusOK, map[string]any{"providers": a.providers.Supported()})
}

func (a *API) handleMe(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	auth.WriteJSON(w, http.StatusOK, principal)
}

func (a *API) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := a.store.ListTeamAccounts(r.Context(), r.PathValue("teamID"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": accounts})
}

func (a *API) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	var input domain.CreateAccountInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	if input.Provider == "" || input.InstanceURL == "" || input.Username == "" || input.AccessToken == "" {
		http.Error(w, "provider, instance_url, username and access_token are required", http.StatusBadRequest)
		return
	}

	if _, ok := a.providers.Get(strings.ToLower(input.Provider)); !ok {
		http.Error(w, "unsupported provider", http.StatusBadRequest)
		return
	}
	input.Provider = strings.ToLower(input.Provider)

	account, err := a.store.CreateAccount(r.Context(), r.PathValue("teamID"), input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusCreated, account)
}

func (a *API) handleListPosts(w http.ResponseWriter, r *http.Request) {
	posts, err := a.store.ListTeamPosts(r.Context(), r.PathValue("teamID"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": posts})
}

func (a *API) handleGetPost(w http.ResponseWriter, r *http.Request) {
	post, err := a.store.GetScheduledPost(r.Context(), r.PathValue("teamID"), r.PathValue("postID"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	auth.WriteJSON(w, http.StatusOK, post)
}

func (a *API) handleCreatePost(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	var input domain.CreatePostInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	input.Content = sanitizeContent(a.sanitizer, input.Content)
	if input.ScheduledAt.IsZero() {
		input.ScheduledAt = time.Now().UTC()
	}

	validation, err := a.validatePostInput(r, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !validation.Valid {
		auth.WriteJSON(w, http.StatusUnprocessableEntity, validation)
		return
	}

	post, err := a.store.CreateScheduledPost(r.Context(), r.PathValue("teamID"), principal, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusCreated, post)
}

func (a *API) handleUpdatePost(w http.ResponseWriter, r *http.Request) {
	var input domain.CreatePostInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	input.Content = sanitizeContent(a.sanitizer, input.Content)
	validation, err := a.validatePostInput(r, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !validation.Valid {
		auth.WriteJSON(w, http.StatusUnprocessableEntity, validation)
		return
	}

	post, err := a.store.UpdateScheduledPost(r.Context(), r.PathValue("teamID"), r.PathValue("postID"), input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, post)
}

func (a *API) handleDeletePost(w http.ResponseWriter, r *http.Request) {
	if err := a.store.DeleteScheduledPost(r.Context(), r.PathValue("teamID"), r.PathValue("postID")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleCancelPost(w http.ResponseWriter, r *http.Request) {
	if err := a.store.CancelScheduledPost(r.Context(), r.PathValue("teamID"), r.PathValue("postID")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleValidatePost(w http.ResponseWriter, r *http.Request) {
	var input domain.CreatePostInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	input.Content = sanitizeContent(a.sanitizer, input.Content)
	validation, err := a.validatePostInput(r, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	auth.WriteJSON(w, http.StatusOK, validation)
}

func (a *API) validatePostInput(r *http.Request, input domain.CreatePostInput) (validationResponse, error) {
	if err := input.Validate(); err != nil {
		return validationResponse{}, err
	}

	teamID := r.PathValue("teamID")
	accounts, err := a.store.GetAccountsByIDs(r.Context(), teamID, input.TargetAccounts)
	if err != nil {
		return validationResponse{}, err
	}
	if len(accounts) != len(input.TargetAccounts) {
		return validationResponse{}, errors.New("one or more target accounts are missing")
	}

	destinations := make([]destinationInfo, 0, len(accounts))
	maxChars := 0
	for _, account := range accounts {
		providerImpl, ok := a.providers.Get(account.Provider)
		if !ok {
			return validationResponse{}, errors.New("one or more target accounts use an unsupported provider")
		}

		capabilities, err := providerImpl.Capabilities(r.Context(), account)
		if err != nil {
			return validationResponse{}, err
		}
		destinations = append(destinations, destinationInfo{
			AccountID: account.ID,
			Provider:  account.Provider,
			MaxChars:  capabilities.MaxChars,
		})
		if maxChars == 0 || capabilities.MaxChars < maxChars {
			maxChars = capabilities.MaxChars
		}
	}

	slices.SortFunc(destinations, func(a, b destinationInfo) int {
		return strings.Compare(a.AccountID, b.AccountID)
	})

	return validationResponse{
		MaxChars:      maxChars,
		ContentLength: len([]rune(input.Content)),
		Valid:         len([]rune(input.Content)) <= maxChars,
		Destinations:  destinations,
	}, nil
}

func sanitizeContent(policy *bluemonday.Policy, content string) string {
	clean := policy.Sanitize(content)
	return strings.TrimSpace(clean)
}
