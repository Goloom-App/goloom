package api

import (
	"net/http"

	"git.f4mily.net/goloom/internal/aicontext"
	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/textcount"
)

// recentPostContextChars caps each recent post in GET /ai-context.
const recentPostContextChars = 280

func (a *API) handleGetAIContext(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")
	ctx, err := aicontext.Build(r.Context(), a.store, teamID, aicontext.Full())
	if err != nil {
		a.writeError(w, r, "team_profile_not_found", http.StatusNotFound)
		return
	}

	for i := range ctx.RecentPosts {
		ctx.RecentPosts[i].Content = textcount.Truncate(ctx.RecentPosts[i].Content, recentPostContextChars)
	}

	auth.WriteJSON(w, http.StatusOK, ctx)
}
