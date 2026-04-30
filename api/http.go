package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/store"
	"github.com/microcosm-cc/bluemonday"
)

type API struct {
	store     store.Store
	auth      *auth.Service
	providers *provider.Registry
	sanitizer *bluemonday.Policy
	config    config.Config
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

func New(store store.Store, authService *auth.Service, providers *provider.Registry, cfg config.Config) *API {
	return &API{
		store:     store,
		auth:      authService,
		providers: providers,
		sanitizer: bluemonday.UGCPolicy(),
		config:    cfg,
	}
}

func (a *API) Handler(limiter *security.Limiter, allowedOrigins []string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.handleHealth)
	mux.HandleFunc("GET /v1/providers", a.handleProviders)
	mux.HandleFunc("GET /v1/auth/status", a.handleAuthStatus)
	mux.HandleFunc("GET /v1/oauth/mastodon/callback", a.handleMastodonOAuthCallback)

	mux.Handle("GET /v1/me", a.auth.RequireAuth(http.HandlerFunc(a.handleMe)))
	mux.Handle("GET /v1/users", a.auth.RequireAuth(http.HandlerFunc(a.handleListUsers)))
	mux.Handle("GET /v1/teams", a.auth.RequireAuth(http.HandlerFunc(a.handleListTeams)))
	mux.Handle("POST /v1/teams", a.auth.RequireAuth(http.HandlerFunc(a.handleCreateTeam)))
	mux.Handle("GET /v1/admin/users", a.auth.RequireAuth(a.auth.RequireAdmin(http.HandlerFunc(a.handleListUsers))))
	mux.Handle("PATCH /v1/admin/users/{userID}", a.auth.RequireAuth(a.auth.RequireAdmin(http.HandlerFunc(a.handleUpdateUser))))
	mux.Handle("GET /v1/admin/runtime-config", a.auth.RequireAuth(a.auth.RequireAdmin(http.HandlerFunc(a.handleRuntimeConfig))))
	mux.Handle("GET /v1/provider-instances", a.auth.RequireAuth(http.HandlerFunc(a.handleListProviderInstances)))
	mux.Handle("GET /v1/provider-instances/{instanceID}", a.auth.RequireAuth(http.HandlerFunc(a.handleGetProviderInstance)))
	mux.Handle("GET /v1/admin/provider-instances", a.auth.RequireAuth(a.auth.RequireAdmin(http.HandlerFunc(a.handleListProviderInstances))))
	mux.Handle("POST /v1/admin/provider-instances", a.auth.RequireAuth(a.auth.RequireAdmin(http.HandlerFunc(a.handleCreateProviderInstance))))
	mux.Handle("PUT /v1/admin/provider-instances/{instanceID}", a.auth.RequireAuth(a.auth.RequireAdmin(http.HandlerFunc(a.handleUpdateProviderInstance))))
	mux.Handle("GET /v1/teams/{teamID}/members", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleListTeamMembers))))
	mux.Handle("POST /v1/teams/{teamID}/members", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleOwner)(http.HandlerFunc(a.handleAddTeamMember))))
	mux.Handle("DELETE /v1/teams/{teamID}/members/{userID}", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleOwner)(http.HandlerFunc(a.handleRemoveTeamMember))))
	mux.Handle("GET /v1/teams/{teamID}/accounts", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleListAccounts))))
	mux.Handle("POST /v1/teams/{teamID}/accounts/oauth/mastodon/start", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleStartMastodonOAuth))))
	mux.Handle("POST /v1/teams/{teamID}/accounts", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleCreateAccount))))
	mux.Handle("DELETE /v1/teams/{teamID}/accounts/{accountID}", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleDeleteAccount))))
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
	auth.WriteJSON(w, http.StatusOK, map[string]any{"providers": sliceOrEmpty(a.providers.Supported())})
}

func (a *API) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	users, err := a.store.ListUsers(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	hasAdminUsers := false
	for _, user := range users {
		if user.IsAdmin {
			hasAdminUsers = true
			break
		}
	}

	auth.WriteJSON(w, http.StatusOK, map[string]any{
		"bootstrap_enabled": a.config.BootstrapAdminToken != "",
		"oidc_enabled":      a.config.OIDCIssuerURL != "" && a.config.OIDCClientID != "",
		"has_users":         len(users) > 0,
		"has_admin_users":   hasAdminUsers,
	})
}

func (a *API) handleMe(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	auth.WriteJSON(w, http.StatusOK, principal)
}

func (a *API) handleListTeams(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	teams, err := a.store.ListTeamsForUser(r.Context(), principal.User.ID, principal.User.IsAdmin)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": sliceOrEmpty(teams)})
}

func (a *API) handleCreateTeam(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	var input domain.CreateTeamInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	if input.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	team, err := a.store.CreateTeam(r.Context(), principal.User.ID, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusCreated, team)
}

func (a *API) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := a.store.ListUsers(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": sliceOrEmpty(users)})
}

func (a *API) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	var input domain.UpdateUserInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	user, err := a.store.SetUserAdmin(r.Context(), r.PathValue("userID"), input.IsAdmin)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, user)
}

func (a *API) handleRuntimeConfig(w http.ResponseWriter, _ *http.Request) {
	auth.WriteJSON(w, http.StatusOK, map[string]any{
		"general": map[string]any{
			"http_addr": a.config.HTTPAddr,
		},
		"security": map[string]any{
			"allowed_origins":       a.config.AllowedOrigins,
			"rate_limit_per_minute": a.config.RateLimitPerMinute,
			"encryption_configured": a.config.EncryptionKey != "",
		},
		"scheduler": map[string]any{
			"poll_interval": a.config.SchedulerPollInterval.String(),
			"workers":       a.config.SchedulerWorkers,
		},
		"oidc": map[string]any{
			"enabled":    a.config.OIDCIssuerURL != "" && a.config.OIDCClientID != "",
			"issuer_url": a.config.OIDCIssuerURL,
			"client_id":  a.config.OIDCClientID,
			"has_secret": a.config.OIDCClientSecret != "",
		},
	})
}

func (a *API) handleListProviderInstances(w http.ResponseWriter, r *http.Request) {
	providerName := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("provider")))
	if providerName != "" {
		if _, ok := a.providers.Get(providerName); !ok {
			http.Error(w, "unsupported provider", http.StatusBadRequest)
			return
		}
	}

	items, err := a.store.ListProviderInstances(r.Context(), providerName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": sliceOrEmpty(items)})
}

func (a *API) handleGetProviderInstance(w http.ResponseWriter, r *http.Request) {
	instance, err := a.store.GetProviderInstanceByID(r.Context(), r.PathValue("instanceID"))
	if err != nil {
		http.Error(w, "provider instance not found", http.StatusNotFound)
		return
	}
	auth.WriteJSON(w, http.StatusOK, instance)
}

func (a *API) handleCreateProviderInstance(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	var input domain.CreateProviderInstanceInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	prepared, err := a.prepareProviderInstance(r, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	instance, err := a.store.CreateProviderInstance(r.Context(), principal.User.ID, prepared)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusCreated, instance)
}

func (a *API) handleUpdateProviderInstance(w http.ResponseWriter, r *http.Request) {
	existing, err := a.store.GetProviderInstanceByID(r.Context(), r.PathValue("instanceID"))
	if err != nil {
		http.Error(w, "provider instance not found", http.StatusNotFound)
		return
	}

	var input domain.CreateProviderInstanceInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(input.Provider) == "" {
		input.Provider = existing.Provider
	}
	if strings.TrimSpace(input.Name) == "" {
		input.Name = existing.Name
	}
	if strings.TrimSpace(input.InstanceURL) == "" {
		input.InstanceURL = existing.InstanceURL
	}
	if strings.TrimSpace(input.ClientID) == "" {
		input.ClientID = existing.ClientID
	}
	if len(input.Scopes) == 0 {
		input.Scopes = existing.Scopes
	}
	if strings.TrimSpace(input.AuthorizationEndpoint) == "" {
		input.AuthorizationEndpoint = existing.AuthorizationEndpoint
	}
	if strings.TrimSpace(input.TokenEndpoint) == "" {
		input.TokenEndpoint = existing.TokenEndpoint
	}
	if strings.TrimSpace(input.ClientSecret) == "" && existing.HasClientSecret {
		clientSecret, err := a.store.DecryptProviderInstanceClientSecret(existing)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		input.ClientSecret = clientSecret
	}

	prepared, err := a.prepareProviderInstance(r, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	instance, err := a.store.UpdateProviderInstance(r.Context(), existing.ID, prepared)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, instance)
}

func (a *API) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := a.store.ListTeamAccounts(r.Context(), r.PathValue("teamID"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": sliceOrEmpty(accounts)})
}

func (a *API) handleListTeamMembers(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.ListTeamMembers(r.Context(), r.PathValue("teamID"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": sliceOrEmpty(items)})
}

func (a *API) handleAddTeamMember(w http.ResponseWriter, r *http.Request) {
	var input domain.AddTeamMemberInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	if input.UserID == "" {
		http.Error(w, "user_id is required", http.StatusBadRequest)
		return
	}
	if !slices.Contains([]domain.TeamRole{domain.RoleOwner, domain.RoleEditor, domain.RoleViewer}, input.Role) {
		http.Error(w, "role must be one of owner, editor, viewer", http.StatusBadRequest)
		return
	}

	membership, err := a.store.AddTeamMember(r.Context(), r.PathValue("teamID"), input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusCreated, membership)
}

func (a *API) handleRemoveTeamMember(w http.ResponseWriter, r *http.Request) {
	if err := a.store.RemoveTeamMember(r.Context(), r.PathValue("teamID"), r.PathValue("userID")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	var input domain.CreateAccountInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	input.Provider = strings.ToLower(strings.TrimSpace(input.Provider))
	providerInstance, err := a.resolveProviderInstance(r, input.ProviderInstanceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if input.Provider == "" && providerInstance != nil {
		input.Provider = providerInstance.Provider
	}
	if input.Provider == "" {
		http.Error(w, "provider is required", http.StatusBadRequest)
		return
	}
	if providerInstance != nil && providerInstance.Provider != input.Provider {
		http.Error(w, "provider_instance_id does not match provider", http.StatusBadRequest)
		return
	}

	providerImpl, ok := a.providers.Get(input.Provider)
	if !ok {
		http.Error(w, "unsupported provider", http.StatusBadRequest)
		return
	}

	connectedAccount, err := providerImpl.ConnectAccount(r.Context(), input, providerInstance)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	account, err := a.store.CreateAccount(r.Context(), r.PathValue("teamID"), connectedAccount)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusCreated, account)
}

func (a *API) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	if err := a.store.DeleteAccount(r.Context(), r.PathValue("teamID"), r.PathValue("accountID")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleListPosts(w http.ResponseWriter, r *http.Request) {
	posts, err := a.store.ListTeamPosts(r.Context(), r.PathValue("teamID"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := a.attachPublishedLinks(r, posts); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": sliceOrEmpty(posts)})
}

func (a *API) handleGetPost(w http.ResponseWriter, r *http.Request) {
	post, err := a.store.GetScheduledPost(r.Context(), r.PathValue("teamID"), r.PathValue("postID"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := a.attachPublishedLinks(r, []domain.ScheduledPost{post}); err == nil {
		postLinks, linksErr := a.store.LoadPublishedLinksByPostIDs(r.Context(), []string{post.ID})
		if linksErr == nil {
			post.PublishedLinks = postLinks[post.ID]
		}
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
	input.Title = strings.TrimSpace(input.Title)
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
	input.Title = strings.TrimSpace(input.Title)
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

func (a *API) attachPublishedLinks(r *http.Request, posts []domain.ScheduledPost) error {
	if len(posts) == 0 {
		return nil
	}
	postIDs := make([]string, 0, len(posts))
	for _, post := range posts {
		postIDs = append(postIDs, post.ID)
	}
	links, err := a.store.LoadPublishedLinksByPostIDs(r.Context(), postIDs)
	if err != nil {
		return err
	}
	for idx := range posts {
		posts[idx].PublishedLinks = links[posts[idx].ID]
	}
	return nil
}

func (a *API) resolveProviderInstance(r *http.Request, instanceID string) (*domain.ProviderInstance, error) {
	if strings.TrimSpace(instanceID) == "" {
		return nil, nil
	}
	instance, err := a.store.GetProviderInstanceByID(r.Context(), strings.TrimSpace(instanceID))
	if err != nil {
		return nil, errors.New("provider_instance_id is invalid")
	}
	return &instance, nil
}

func (a *API) prepareProviderInstance(r *http.Request, input domain.CreateProviderInstanceInput) (domain.PreparedProviderInstance, error) {
	input.Provider = strings.ToLower(strings.TrimSpace(input.Provider))
	if input.Provider == "" {
		return domain.PreparedProviderInstance{}, errors.New("provider is required")
	}

	providerImpl, ok := a.providers.Get(input.Provider)
	if !ok {
		return domain.PreparedProviderInstance{}, errors.New("unsupported provider")
	}
	return providerImpl.PrepareProviderInstance(r.Context(), input)
}

func sliceOrEmpty[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}
