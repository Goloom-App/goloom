package api

import (
	"net/http"
	"strings"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/socialtokens"
)

type mediaUploadResponse struct {
	MediaID string `json:"media_id"`
}

func (a *API) handleTeamMediaUpload(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	teamID := r.PathValue("teamID")
	allowed, err := a.store.UserHasAnyTeamRole(r.Context(), principal.User.ID, teamID, domain.RoleEditor, domain.RoleOwner)
	if err != nil || !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "invalid multipart form", http.StatusBadRequest)
		return
	}
	accountID := strings.TrimSpace(r.FormValue("account_id"))
	if accountID == "" {
		http.Error(w, "account_id is required", http.StatusBadRequest)
		return
	}
	account, err := a.store.GetAccountByID(r.Context(), accountID)
	if err != nil {
		http.Error(w, "account not found", http.StatusBadRequest)
		return
	}
	if account.TeamID != teamID {
		http.Error(w, "account not in team", http.StatusBadRequest)
		return
	}

	file, hdr, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	providerImpl, ok := a.providers.Get(account.Provider)
	if !ok {
		http.Error(w, "unsupported provider", http.StatusBadRequest)
		return
	}

	acc, err := socialtokens.EnsureMastodonFresh(r.Context(), a.store, a.providers, account)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	account = acc

	token, err := a.store.DecryptAccessToken(account)
	if err != nil {
		http.Error(w, "failed to read account credentials", http.StatusInternalServerError)
		return
	}
	refreshToken, err := a.store.DecryptRefreshToken(account)
	if err != nil {
		http.Error(w, "failed to read account credentials", http.StatusInternalServerError)
		return
	}

	filename := hdr.Filename
	if strings.TrimSpace(filename) == "" {
		filename = "upload"
	}
	mimeType := hdr.Header.Get("Content-Type")
	altText := strings.TrimSpace(r.FormValue("alt_text"))

	mediaID, err := providerImpl.UploadMedia(r.Context(), account, provider.PublishAuth{
		AccessToken:  token,
		RefreshToken: refreshToken,
	}, file, filename, mimeType, altText)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	auth.WriteJSON(w, http.StatusOK, mediaUploadResponse{MediaID: mediaID})
}
