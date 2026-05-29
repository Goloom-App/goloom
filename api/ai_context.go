package api

import (
	"net/http"

	"git.f4mily.net/goloom/internal/auth"
)

func (a *API) handleGetAIContext(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")
	ctx, err := a.store.GetTeamAIContext(r.Context(), teamID)
	if err != nil {
		a.writeError(w, r, "team_profile_not_found", http.StatusNotFound)
		return
	}

	for i := range ctx.RecentPosts {
		if len(ctx.RecentPosts[i].Content) > 280 {
			ctx.RecentPosts[i].Content = ctx.RecentPosts[i].Content[len(ctx.RecentPosts[i].Content)-280:]
		}
	}

	auth.WriteJSON(w, http.StatusOK, ctx)
}
