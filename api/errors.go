package api

import "net/http"

func (a *API) writeError(w http.ResponseWriter, r *http.Request, key string, status int) {
	if a.i18n != nil {
		a.i18n.WriteError(w, r, key, status)
		return
	}
	http.Error(w, key, status)
}
