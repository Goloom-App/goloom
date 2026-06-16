package api

import (
	"encoding/json"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"strings"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/store"
)

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
		a.writeError(w, r, "invalid_multipart_form", http.StatusBadRequest)
		return
	}

	file, hdr, err := r.FormFile("file")
	if err != nil {
		a.writeError(w, r, "file_required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	hash, size, err := store.SaveMediaFile(teamID, file)
	if err != nil {
		http.Error(w, "failed to save media: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if existing, ok, qerr := a.store.FindMediaItemByTeamSHA256(r.Context(), teamID, hash); qerr != nil {
		http.Error(w, qerr.Error(), http.StatusInternalServerError)
		return
	} else if ok {
		auth.WriteJSON(w, http.StatusOK, existing)
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
		if path, err := store.GetMediaFilePath(teamID, hash); err == nil {
			// path is contained within the media root by GetMediaFilePath.
			if f, err := os.Open(path); err == nil { // #nosec G703 -- path validated/contained by store.GetMediaFilePath
				if cfg, _, err := image.DecodeConfig(f); err == nil {
					item.Width = &cfg.Width
					item.Height = &cfg.Height
				}
				f.Close()
			}
		}
	}

	created, err := a.store.CreateMediaItem(r.Context(), item)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	auth.WriteJSON(w, http.StatusCreated, created)
}

func (a *API) handleTeamMediaRename(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")
	mediaID := r.PathValue("mediaID")

	var input struct {
		Filename string `json:"filename"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	filename := strings.TrimSpace(input.Filename)
	if filename == "" {
		a.writeError(w, r, "filename_required", http.StatusBadRequest)
		return
	}
	if len([]rune(filename)) > 255 {
		a.writeError(w, r, "filename_too_long", http.StatusBadRequest)
		return
	}

	if _, err := a.store.GetMediaItemByID(r.Context(), teamID, mediaID); err != nil {
		a.writeError(w, r, "not_found", http.StatusNotFound)
		return
	}

	updated, err := a.store.UpdateMediaItemFilename(r.Context(), teamID, mediaID, filename)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.recordAudit(r, teamID, "media.rename", "media", &mediaID, "Renamed media to "+filename)
	auth.WriteJSON(w, http.StatusOK, updated)
}

func (a *API) handleTeamMediaDelete(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")
	mediaID := r.PathValue("mediaID")

	item, err := a.store.GetMediaItemByID(r.Context(), teamID, mediaID)
	if err != nil {
		a.writeError(w, r, "not_found", http.StatusNotFound)
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
		a.writeError(w, r, "not_found", http.StatusNotFound)
		return
	}

	filePath, err := store.GetMediaFilePath(teamID, item.Sha256)
	if err != nil {
		a.writeError(w, r, "not_found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", item.MimeType)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	// filePath is contained within the media root by GetMediaFilePath.
	http.ServeFile(w, r, filePath) // #nosec G703 -- path validated/contained by store.GetMediaFilePath
}
