package auth

import (
	"net/http"
)

// RequireAIEnabled returns middleware that checks team.is_ai_enabled=true.
// teamIDParam is the URL path parameter name (e.g., "teamID").
func (s *Service) RequireAIEnabled(teamIDParam string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal := PrincipalFromContext(r.Context())
			if principal == nil {
				http.Error(w, "missing principal", http.StatusUnauthorized)
				return
			}

			teamID := r.PathValue(teamIDParam)
			team, err := s.store.GetTeamByID(r.Context(), teamID)
			if err != nil {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			if !team.IsAIEnabled {
				http.Error(w, "ai_not_enabled", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
