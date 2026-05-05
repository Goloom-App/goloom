package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/store"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type Service struct {
	store     store.Store
	verifier  *oidc.IDTokenVerifier
	oauth2Cfg oauth2.Config
}

type oidcClaims struct {
	Subject string `json:"sub"`
	Email   string `json:"email"`
	Name    string `json:"name"`
}

func New(ctx context.Context, cfg config.Config, store store.Store) (*Service, error) {
	service := &Service{store: store}
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
// upserts the user, and returns the raw ID token JWT for use as an API bearer token.
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
	return raw, domain.AuthenticatedPrincipal{User: user, Kind: "oidc"}, nil
}

func (s *Service) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r.Header.Get("Authorization"))
		if token == "" {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}

		principal, err := s.authenticate(r.Context(), token)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := security.WithPrincipal(r.Context(), principal)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Service) RequireTeamRole(teamIDParam string, roles ...domain.TeamRole) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := security.PrincipalFromContext[domain.AuthenticatedPrincipal](r.Context())
			if !ok {
				http.Error(w, "missing principal", http.StatusUnauthorized)
				return
			}

			teamID := r.PathValue(teamIDParam)
			allowed, err := s.store.UserHasAnyTeamRole(r.Context(), principal.User.ID, teamID, roles...)
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
		principal, ok := security.PrincipalFromContext[domain.AuthenticatedPrincipal](r.Context())
		if !ok {
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

	return s.store.LookupAPIToken(ctx, token)
}

func (s *Service) CurrentPrincipal(r *http.Request) (domain.AuthenticatedPrincipal, error) {
	principal, ok := security.PrincipalFromContext[domain.AuthenticatedPrincipal](r.Context())
	if !ok {
		return domain.AuthenticatedPrincipal{}, errors.New("missing principal")
	}
	return principal, nil
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
