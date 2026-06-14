package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/store"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type Service struct {
	store         store.Store
	verifier      *oidc.IDTokenVerifier
	oauth2Cfg     oauth2.Config
	sessionTTL    time.Duration
	secureCookies bool
}

type oidcClaims struct {
	Subject string `json:"sub"`
	Email   string `json:"email"`
	Name    string `json:"name"`
}

func New(ctx context.Context, cfg config.Config, store store.Store) (*Service, error) {
	service := &Service{
		store:      store,
		sessionTTL: cfg.SessionTTL,
		// Secure cookies whenever the app is served over HTTPS; relaxed for local
		// http dev so the cookie is not dropped by the browser.
		secureCookies: strings.HasPrefix(strings.ToLower(strings.TrimSpace(cfg.PublicBaseURL)), "https://"),
	}
	if service.sessionTTL <= 0 {
		service.sessionTTL = 12 * time.Hour
	}
	if cfg.OIDCIssuerURL == "" || cfg.OIDCClientID == "" {
		return service, nil
	}

	provider, err := oidc.NewProvider(ctx, cfg.OIDCIssuerURL)
	if err != nil {
		return nil, err
	}
	verifier, err := buildVerifierFromProvider(ctx, provider, cfg.OIDCClientID, cfg.OIDCClientSecret)
	if err != nil {
		return nil, err
	}
	service.verifier = verifier
	service.oauth2Cfg = oauth2.Config{
		ClientID:     cfg.OIDCClientID,
		ClientSecret: cfg.OIDCClientSecret,
		RedirectURL:  cfg.OIDCRedirectURI,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}
	return service, nil
}

// OIDCOAuthReady reports whether browser-based OIDC authorization (authorization code + PKCE) can run.
func (s *Service) OIDCOAuthReady() bool {
	if s == nil || s.verifier == nil {
		return false
	}
	return strings.TrimSpace(s.oauth2Cfg.ClientID) != "" && strings.TrimSpace(s.oauth2Cfg.RedirectURL) != ""
}

// OIDCAuthCodeURL builds the IdP authorization URL for the OIDC login redirect flow.
func (s *Service) OIDCAuthCodeURL(state, nonce, pkceVerifier string) (string, error) {
	if !s.OIDCOAuthReady() {
		return "", errors.New("oidc oauth is not configured")
	}
	return s.oauth2Cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oidc.Nonce(nonce), oauth2.S256ChallengeOption(pkceVerifier)), nil
}

// OIDCExchangeCode completes the authorization code flow, verifies the ID token (including nonce),
// upserts the user, and returns a rolling API session token for SPA bearer auth.
func (s *Service) OIDCExchangeCode(ctx context.Context, code, nonce, pkceVerifier string) (rawIDToken string, principal domain.AuthenticatedPrincipal, err error) {
	if !s.OIDCOAuthReady() {
		return "", domain.AuthenticatedPrincipal{}, errors.New("oidc oauth is not configured")
	}
	tok, err := s.oauth2Cfg.Exchange(ctx, code, oauth2.VerifierOption(pkceVerifier))
	if err != nil {
		return "", domain.AuthenticatedPrincipal{}, fmt.Errorf("oauth2 Exchange (token endpoint): %w", err)
	}
	raw, ok := tok.Extra("id_token").(string)
	if !ok || raw == "" {
		return "", domain.AuthenticatedPrincipal{}, errors.New("token response did not include id_token (need openid scope and IdP that returns id_token)")
	}
	idToken, err := s.verifier.Verify(ctx, raw)
	if err != nil {
		return "", domain.AuthenticatedPrincipal{}, fmt.Errorf("id_token Verify (issuer/audience/signature): %w", err)
	}
	if nonce != "" && idToken.Nonce != nonce {
		return "", domain.AuthenticatedPrincipal{}, errors.New("id token nonce mismatch")
	}
	var claims oidcClaims
	if err := idToken.Claims(&claims); err != nil {
		return "", domain.AuthenticatedPrincipal{}, fmt.Errorf("id_token Claims: %w", err)
	}
	user, err := s.store.UpsertOIDCUser(ctx, claims.Subject, claims.Email, claims.Name)
	if err != nil {
		return "", domain.AuthenticatedPrincipal{}, fmt.Errorf("UpsertOIDCUser: %w", err)
	}
	sessionToken, _, err := s.store.CreateSessionAPIToken(ctx, user.ID, 12*time.Hour)
	if err != nil {
		return "", domain.AuthenticatedPrincipal{}, fmt.Errorf("CreateSessionAPIToken: %w", err)
	}
	return sessionToken, domain.AuthenticatedPrincipal{User: user, Kind: "oidc"}, nil
}

func (s *Service) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r.Header.Get("Authorization"))
		fromCookie := false
		if token == "" {
			// Browser sessions authenticate via the HttpOnly cookie. API tokens
			// (MCP, tools) always use the Authorization header above.
			if c := strings.TrimSpace(cookieValue(r, SessionCookieName)); c != "" {
				token = c
				fromCookie = true
			}
		}
		if token == "" {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}

		principal, err := s.authenticate(r.Context(), token)
		if err != nil {
			// Only a genuine "no such token/session" (ErrNoRows) means the caller is
			// unauthenticated. A transient backend failure (DB contention, context
			// cancellation, connection loss) must surface as 500 — returning 401 here
			// would make the web UI treat a hiccup as an expired session and log out.
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
			} else {
				http.Error(w, "authentication unavailable", http.StatusInternalServerError)
			}
			return
		}

		if fromCookie {
			// CSRF: cookie auth on a state-changing method needs the double-submit
			// token. Bearer (header) auth is exempt — it cannot be sent cross-site.
			if isUnsafeMethod(r.Method) && !csrfValid(r) {
				http.Error(w, "csrf_token_required", http.StatusForbidden)
				return
			}
			// Roll the cookie (the DB expiry was just rolled in authenticate);
			// keep the existing CSRF token, minting one if a legacy session lacks it.
			csrf := cookieValue(r, CSRFCookieName)
			if csrf == "" {
				if generated, gerr := NewCSRFToken(); gerr == nil {
					csrf = generated
				}
			}
			s.WriteSessionCookies(w, token, csrf)
		}

		ctx := security.WithPrincipal(r.Context(), principal)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Service) AcceptQueryToken(param string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if bearerToken(r.Header.Get("Authorization")) == "" {
				if token := strings.TrimSpace(r.URL.Query().Get(param)); token != "" {
					clone := r.Clone(r.Context())
					clone.Header = r.Header.Clone()
					clone.Header.Set("Authorization", "Bearer "+token)
					r = clone
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// PrincipalHasTeamAccess reports whether the caller may access the team:
// global administrators may use any team (recovery / OIDC-linked data); otherwise
// a team_memberships role is required.
func (s *Service) PrincipalHasTeamAccess(ctx context.Context, principal domain.AuthenticatedPrincipal, teamID string, roles ...domain.TeamRole) (bool, error) {
	// A team-bound API token may only ever touch its own team — the token itself
	// is the limit, so this applies even to administrators.
	if principal.TokenTeamID != nil {
		if bound := strings.TrimSpace(*principal.TokenTeamID); bound != "" && bound != teamID {
			return false, nil
		}
	}
	if principal.User.IsAdmin {
		return true, nil
	}
	return s.store.UserHasAnyTeamRole(ctx, principal.User.ID, teamID, roles...)
}

func (s *Service) RequireTeamRole(teamIDParam string, roles ...domain.TeamRole) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal := PrincipalFromContext(r.Context())
			if principal == nil {
				http.Error(w, "missing principal", http.StatusUnauthorized)
				return
			}

			teamID := r.PathValue(teamIDParam)
			allowed, err := s.PrincipalHasTeamAccess(r.Context(), *principal, teamID, roles...)
			if err != nil {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			if !allowed {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func (s *Service) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal := PrincipalFromContext(r.Context())
		if principal == nil {
			http.Error(w, "missing principal", http.StatusUnauthorized)
			return
		}
		if !principal.User.IsAdmin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Service) authenticate(ctx context.Context, token string) (domain.AuthenticatedPrincipal, error) {
	if strings.Count(token, ".") == 2 && s.verifier != nil {
		idToken, err := s.verifier.Verify(ctx, token)
		if err == nil {
			var claims oidcClaims
			if err := idToken.Claims(&claims); err != nil {
				return domain.AuthenticatedPrincipal{}, err
			}
			user, err := s.store.UpsertOIDCUser(ctx, claims.Subject, claims.Email, claims.Name)
			if err != nil {
				return domain.AuthenticatedPrincipal{}, err
			}
			return domain.AuthenticatedPrincipal{User: user, Kind: "oidc"}, nil
		}
	}

	principal, err := s.store.LookupAPIToken(ctx, token)
	if err != nil {
		return domain.AuthenticatedPrincipal{}, err
	}
	if principal.TokenTeamID != nil {
		teamID := strings.TrimSpace(*principal.TokenTeamID)
		if teamID == "" {
			principal.TokenTeamID = nil
		} else {
			principal.TokenTeamID = &teamID
		}
	}
	return principal, nil
}

func (s *Service) CurrentPrincipal(r *http.Request) (domain.AuthenticatedPrincipal, error) {
	principal := PrincipalFromContext(r.Context())
	if principal == nil {
		return domain.AuthenticatedPrincipal{}, errors.New("missing principal")
	}
	return *principal, nil
}

func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func bearerToken(header string) string {
	if !strings.HasPrefix(header, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
}
