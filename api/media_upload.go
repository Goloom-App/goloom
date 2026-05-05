package api

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"strings"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/socialtokens"
	"git.f4mily.net/goloom/internal/store"
)

type mediaUploadResponse struct {
	MediaID string `json:"media_id"`
}

func (a *API) handleTeamMediaList(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")
	items, err := a.store.ListTeamMedia(r.Context(), teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": sliceOrEmpty(items)})
}

func (a *API) handleTeamMediaUploadToLibrary(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "invalid multipart form", http.StatusBadRequest)
		return
	}

	file, hdr, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	hash, size, err := store.SaveMediaFile(teamID, file)
	if err != nil {
		http.Error(w, "failed to save media: "+err.Error(), http.StatusInternalServerError)
		return
	}

	filename := hdr.Filename
	if strings.TrimSpace(filename) == "" {
		filename = "upload"
	}
	mimeType := hdr.Header.Get("Content-Type")

	item := domain.MediaItem{
		TeamID:    teamID,
		Sha256:    hash,
		Filename:  filename,
		MimeType:  mimeType,
		SizeBytes: size,
	}

	// Try to get dimensions if it's an image
	if strings.HasPrefix(mimeType, "image/") {
		if f, err := os.Open(store.GetMediaFilePath(teamID, hash)); err == nil {
			if cfg, _, err := image.DecodeConfig(f); err == nil {
				item.Width = &cfg.Width
				item.Height = &cfg.Height
			}
			f.Close()
		}
	}

	created, err := a.store.CreateMediaItem(r.Context(), item)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	auth.WriteJSON(w, http.StatusCreated, created)
}

func (a *API) handleTeamMediaDelete(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")
	mediaID := r.PathValue("mediaID")

	item, err := a.store.GetMediaItemByID(r.Context(), teamID, mediaID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if err := a.store.DeleteMediaItem(r.Context(), teamID, mediaID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Best effort cleanup of file
	_ = store.DeleteMediaFile(teamID, item.Sha256)

	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleTeamMediaPreview(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")
	mediaID := r.PathValue("mediaID")

	item, err := a.store.GetMediaItemByID(r.Context(), teamID, mediaID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	filePath := store.GetMediaFilePath(teamID, item.Sha256)
	w.Header().Set("Content-Type", item.MimeType)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeFile(w, r, filePath)
}

// handleTeamMediaUpload remains for legacy direct provider uploads (Phase 6 will remove it)
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
