package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/store/postgres"
	"github.com/coreos/go-oidc/v3/oidc"
)

type Service struct {
	store    *postgres.Store
	verifier *oidc.IDTokenVerifier
}

type oidcClaims struct {
	Subject string `json:"sub"`
	Email   string `json:"email"`
	Name    string `json:"name"`
}

func New(ctx context.Context, cfg config.Config, store *postgres.Store) (*Service, error) {
	service := &Service{store: store}
	if cfg.OIDCIssuerURL == "" || cfg.OIDCClientID == "" {
		return service, nil
	}

	provider, err := oidc.NewProvider(ctx, cfg.OIDCIssuerURL)
	if err != nil {
		return nil, err
	}
	service.verifier = provider.Verifier(&oidc.Config{ClientID: cfg.OIDCClientID})
	return service, nil
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
