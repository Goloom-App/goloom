package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
)

const mastodonOAuthStateTTL = 10 * time.Minute

// mastodonOAuthProviders are providers that authenticate via the Mastodon OAuth app flow
// and share the /v1/oauth/mastodon/callback redirect (Pixelfed is Mastodon API-compatible).
var mastodonOAuthProviders = map[string]bool{
	"mastodon": true,
	"pixelfed": true,
}

func isMastodonCompatibleOAuthProvider(name string) bool {
	return mastodonOAuthProviders[strings.ToLower(strings.TrimSpace(name))]
}

// titleCaseProvider upper-cases the first letter of an ASCII provider name (e.g. "pixelfed" -> "Pixelfed").
func titleCaseProvider(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return name
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

type startMastodonOAuthRequest struct {
	ProviderInstanceID string `json:"provider_instance_id"`
	ReturnTo           string `json:"return_to"`
}

type oauthAuthorizationResponse struct {
	AuthorizationURL string `json:"authorization_url"`
}

type mastodonOAuthState struct {
	Version            int    `json:"v"`
	Provider           string `json:"p"`
	UserID             string `json:"u"`
	TeamID             string `json:"t"`
	ProviderInstanceID string `json:"i"`
	ReturnTo           string `json:"r"`
	ExpiresAtUnix      int64  `json:"e"`
}

func (a *API) handleStartMastodonOAuth(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	var input startMastodonOAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}

	instance, err := a.store.GetProviderInstanceByID(r.Context(), strings.TrimSpace(input.ProviderInstanceID))
	if err != nil {
		a.writeError(w, r, "provider_instance_id_invalid", http.StatusBadRequest)
		return
	}
	if !isMastodonCompatibleOAuthProvider(instance.Provider) {
		a.writeError(w, r, "provider_instance_must_be_mastodon", http.StatusBadRequest)
		return
	}

	providerImpl, ok := a.providers.Get(instance.Provider)
	if !ok {
		a.writeError(w, r, "unsupported_provider", http.StatusBadRequest)
		return
	}
	connector, ok := providerImpl.(provider.OAuthAccountConnector)
	if !ok {
		a.writeError(w, r, "provider_no_oauth", http.StatusBadRequest)
		return
	}

	returnTo, err := a.normalizeOAuthReturnURL(input.ReturnTo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	state, err := a.signMastodonOAuthState(mastodonOAuthState{
		Version:            1,
		Provider:           instance.Provider,
		UserID:             principal.User.ID,
		TeamID:             r.PathValue("teamID"),
		ProviderInstanceID: instance.ID,
		ReturnTo:           returnTo,
		ExpiresAtUnix:      time.Now().UTC().Add(mastodonOAuthStateTTL).Unix(),
	})
	if err != nil {
		a.writeError(w, r, "failed_build_oauth_state", http.StatusInternalServerError)
		return
	}

	authorizationURL, err := connector.BuildAuthorizationURL(instance, state, a.config.MastodonRedirectURI)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	auth.WriteJSON(w, http.StatusOK, oauthAuthorizationResponse{AuthorizationURL: authorizationURL})
}

func (a *API) handleMastodonOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if providerError := strings.TrimSpace(r.URL.Query().Get("error")); providerError != "" {
		description := strings.TrimSpace(r.URL.Query().Get("error_description"))
		message := providerError
		if description != "" {
			message = description
		}
		a.redirectOAuthError(w, r, strings.TrimSpace(r.URL.Query().Get("state")), message)
		return
	}

	state, err := a.parseMastodonOAuthState(strings.TrimSpace(r.URL.Query().Get("state")))
	if err != nil {
		a.writeError(w, r, "invalid_oauth_state", http.StatusBadRequest)
		return
	}

	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		a.redirectOAuthResult(w, r, state.ReturnTo, "error", state.Provider, "missing authorization code")
		return
	}

	allowed, err := a.store.UserHasAnyTeamRole(r.Context(), state.UserID, state.TeamID, domain.RoleEditor, domain.RoleOwner)
	if err != nil {
		a.redirectOAuthResult(w, r, state.ReturnTo, "error", state.Provider, "failed to validate team membership")
		return
	}
	if !allowed {
		a.redirectOAuthResult(w, r, state.ReturnTo, "error", state.Provider, "team access was revoked before the oauth callback completed")
		return
	}

	instance, err := a.store.GetProviderInstanceByID(r.Context(), state.ProviderInstanceID)
	if err != nil {
		a.redirectOAuthResult(w, r, state.ReturnTo, "error", state.Provider, "provider instance is no longer available")
		return
	}
	if !isMastodonCompatibleOAuthProvider(instance.Provider) {
		a.redirectOAuthResult(w, r, state.ReturnTo, "error", state.Provider, "provider instance no longer supports mastodon oauth")
		return
	}

	providerImpl, ok := a.providers.Get(instance.Provider)
	if !ok {
		a.redirectOAuthResult(w, r, state.ReturnTo, "error", state.Provider, "unsupported provider")
		return
	}
	connector, ok := providerImpl.(provider.OAuthAccountConnector)
	if !ok {
		a.redirectOAuthResult(w, r, state.ReturnTo, "error", state.Provider, "provider does not support oauth")
		return
	}

	clientSecret, err := a.store.DecryptProviderInstanceClientSecret(instance)
	if err != nil {
		a.redirectOAuthResult(w, r, state.ReturnTo, "error", state.Provider, "failed to load provider credentials")
		return
	}
	if strings.TrimSpace(clientSecret) == "" {
		a.redirectOAuthResult(w, r, state.ReturnTo, "error", state.Provider, "provider instance is missing a client secret")
		return
	}

	accountInput, err := connector.ConnectAccountOAuthCallback(a.providerContext(r.Context()), instance, clientSecret, a.config.MastodonRedirectURI, code)
	if err != nil {
		a.redirectOAuthResult(w, r, state.ReturnTo, "error", state.Provider, err.Error())
		return
	}

	account, err := a.store.CreateAccount(r.Context(), state.TeamID, accountInput)
	if err != nil {
		a.redirectOAuthResult(w, r, state.ReturnTo, "error", state.Provider, "failed to save account")
		return
	}

	message := fmt.Sprintf("Connected %s account %s", titleCaseProvider(state.Provider), account.Username)
	a.redirectOAuthResult(w, r, state.ReturnTo, "success", state.Provider, message)
}

func (a *API) redirectOAuthError(w http.ResponseWriter, r *http.Request, rawState, message string) {
	state, err := a.parseMastodonOAuthState(rawState)
	if err != nil {
		http.Error(w, message, http.StatusBadRequest)
		return
	}
	a.redirectOAuthResult(w, r, state.ReturnTo, "error", state.Provider, message)
}

func (a *API) redirectOAuthResult(w http.ResponseWriter, r *http.Request, returnTo, status, providerName, message string) {
	redirectURL, err := appendQueryParams(returnTo, map[string]string{
		"oauth_status":   status,
		"oauth_provider": providerName,
		"oauth_message":  message,
	})
	if err != nil {
		http.Error(w, message, http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

func (a *API) normalizeOAuthReturnURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		trimmed = strings.TrimRight(a.config.PublicBaseURL, "/") + "/"
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", errors.New("return_to must be a valid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("return_to must use http or https")
	}
	if parsed.Host == "" {
		return "", errors.New("return_to must include a host")
	}

	parsed.Fragment = ""
	origin := parsed.Scheme + "://" + parsed.Host
	if !a.isAllowedOAuthOrigin(origin) {
		return "", errors.New("return_to origin is not allowed")
	}
	return parsed.String(), nil
}

func (a *API) isAllowedOAuthOrigin(origin string) bool {
	trimmedOrigin := strings.TrimSpace(origin)
	if trimmedOrigin == "" {
		return false
	}

	publicOrigin := oauthOriginForURL(a.config.PublicBaseURL)
	if publicOrigin != "" && publicOrigin == trimmedOrigin {
		return true
	}

	for _, allowedOrigin := range a.config.AllowedOrigins {
		normalized := strings.TrimSpace(allowedOrigin)
		if normalized == "*" || normalized == trimmedOrigin {
			return true
		}
	}
	return false
}

func (a *API) signMastodonOAuthState(state mastodonOAuthState) (string, error) {
	payload, err := json.Marshal(state)
	if err != nil {
		return "", fmt.Errorf("marshal oauth state: %w", err)
	}

	payloadPart := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, a.oauthStateSecret())
	_, _ = mac.Write([]byte(payloadPart))
	signaturePart := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return payloadPart + "." + signaturePart, nil
}

func (a *API) parseMastodonOAuthState(raw string) (mastodonOAuthState, error) {
	parts := strings.Split(strings.TrimSpace(raw), ".")
	if len(parts) != 2 {
		return mastodonOAuthState{}, errors.New("invalid oauth state format")
	}

	mac := hmac.New(sha256.New, a.oauthStateSecret())
	_, _ = mac.Write([]byte(parts[0]))
	expectedSignature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return mastodonOAuthState{}, errors.New("invalid oauth state signature")
	}
	if !hmac.Equal(mac.Sum(nil), expectedSignature) {
		return mastodonOAuthState{}, errors.New("oauth state signature mismatch")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return mastodonOAuthState{}, errors.New("invalid oauth state payload")
	}

	var state mastodonOAuthState
	if err := json.Unmarshal(payload, &state); err != nil {
		return mastodonOAuthState{}, errors.New("invalid oauth state body")
	}
	if state.Version != 1 {
		return mastodonOAuthState{}, errors.New("unsupported oauth state version")
	}
	if !isMastodonCompatibleOAuthProvider(state.Provider) {
		return mastodonOAuthState{}, errors.New("unexpected oauth provider")
	}
	if state.UserID == "" || state.TeamID == "" || state.ProviderInstanceID == "" || state.ReturnTo == "" {
		return mastodonOAuthState{}, errors.New("oauth state is incomplete")
	}
	if time.Now().UTC().Unix() > state.ExpiresAtUnix {
		return mastodonOAuthState{}, errors.New("oauth state expired")
	}
	if _, err := a.normalizeOAuthReturnURL(state.ReturnTo); err != nil {
		return mastodonOAuthState{}, err
	}
	return state, nil
}

func (a *API) oauthStateSecret() []byte {
	sum := sha256.Sum256([]byte(a.config.EncryptionKey + ":mastodon-oauth-state"))
	return sum[:]
}

func oauthOriginForURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func appendQueryParams(raw string, params map[string]string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}

	query := parsed.Query()
	for key, value := range params {
		query.Set(key, value)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}
