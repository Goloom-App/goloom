package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/auth"
	"golang.org/x/oauth2"
)

const oidcLoginOAuthStateTTL = 10 * time.Minute

type startOIDCLoginRequest struct {
	ReturnTo string `json:"return_to"`
}

type oidcLoginState struct {
	Version       int    `json:"v"`
	ReturnTo      string `json:"r"`
	Nonce         string `json:"n"`
	PKCEVerifier  string `json:"k"`
	ExpiresAtUnix int64  `json:"e"`
}

func (a *API) handleStartOIDCLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !a.auth.OIDCOAuthReady() {
		http.Error(w, "oidc oauth is not configured", http.StatusServiceUnavailable)
		return
	}

	var input startOIDCLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	returnTo, err := a.normalizeOAuthReturnURL(input.ReturnTo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	nonce, err := randomURLSafeString(32)
	if err != nil {
		http.Error(w, "failed to build oauth state", http.StatusInternalServerError)
		return
	}
	pkceVerifier := oauth2.GenerateVerifier()

	state, err := a.signOIDCLoginState(oidcLoginState{
		Version:       1,
		ReturnTo:      returnTo,
		Nonce:         nonce,
		PKCEVerifier:  pkceVerifier,
		ExpiresAtUnix: time.Now().UTC().Add(oidcLoginOAuthStateTTL).Unix(),
	})
	if err != nil {
		http.Error(w, "failed to build oauth state", http.StatusInternalServerError)
		return
	}

	authorizationURL, err := a.auth.OIDCAuthCodeURL(state, nonce, pkceVerifier)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	auth.WriteJSON(w, http.StatusOK, oauthAuthorizationResponse{AuthorizationURL: authorizationURL})
}

func (a *API) handleOIDCLoginCallback(w http.ResponseWriter, r *http.Request) {
	if providerError := strings.TrimSpace(r.URL.Query().Get("error")); providerError != "" {
		description := strings.TrimSpace(r.URL.Query().Get("error_description"))
		message := providerError
		if description != "" {
			message = description
		}
		a.redirectOIDCLoginError(w, r, strings.TrimSpace(r.URL.Query().Get("state")), message)
		return
	}

	rawState := strings.TrimSpace(r.URL.Query().Get("state"))
	state, err := a.parseOIDCLoginState(rawState)
	if err != nil {
		http.Error(w, "invalid oauth state", http.StatusBadRequest)
		return
	}

	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		a.redirectOIDCLoginFailure(w, r, state.ReturnTo, "missing authorization code")
		return
	}

	rawIDToken, _, err := a.auth.OIDCExchangeCode(r.Context(), code, state.Nonce, state.PKCEVerifier)
	if err != nil {
		a.redirectOIDCLoginFailure(w, r, state.ReturnTo, err.Error())
		return
	}

	if err := writeOIDCLoginSuccessHTML(w, state.ReturnTo, rawIDToken); err != nil {
		http.Error(w, "failed to render callback page", http.StatusInternalServerError)
		return
	}
}

// writeOIDCLoginSuccessHTML returns an HTML document that moves the ID token into the URL hash
// via client-side navigation. HTTP redirects with Location + fragment are unreliable across
// browsers and proxies; this avoids losing #goloom_oidc_token=… before the SPA reads it.
func writeOIDCLoginSuccessHTML(w http.ResponseWriter, returnTo, rawIDToken string) error {
	rt, err := json.Marshal(returnTo)
	if err != nil {
		return err
	}
	tk, err := json.Marshal(rawIDToken)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	data := struct {
		ReturnTo template.JS
		Token    template.JS
	}{
		ReturnTo: template.JS(rt),
		Token:    template.JS(tk),
	}
	return oidcLoginSuccessTemplate.Execute(w, data)
}

var oidcLoginSuccessTemplate = template.Must(template.New("oidc-login-success").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Signing in…</title>
</head>
<body>
<p>Signing in… If you are not redirected, <a id="fallback">open the app</a>.</p>
<script>
(function () {
  var returnTo = {{.ReturnTo}};
  var token = {{.Token}};
  try {
    var u = new URL(returnTo);
    u.hash = "goloom_oidc_token=" + encodeURIComponent(token);
    location.replace(u.toString());
  } catch (e) {
    document.body.insertAdjacentHTML("beforeend", "<p>Could not complete redirect: " + String(e) + "</p>");
  }
  var a = document.getElementById("fallback");
  if (a) {
    try { a.href = new URL(returnTo).origin + "/"; } catch (_) { a.href = "/"; }
  }
})();
</script>
</body>
</html>`))

func (a *API) redirectOIDCLoginError(w http.ResponseWriter, r *http.Request, rawState, message string) {
	state, err := a.parseOIDCLoginState(rawState)
	if err != nil {
		http.Error(w, message, http.StatusBadRequest)
		return
	}
	a.redirectOIDCLoginFailure(w, r, state.ReturnTo, message)
}

func (a *API) redirectOIDCLoginFailure(w http.ResponseWriter, r *http.Request, returnTo, message string) {
	redirectURL, err := appendQueryParams(returnTo, map[string]string{
		"oauth_status":   "error",
		"oauth_provider": "oidc",
		"oauth_message":  message,
	})
	if err != nil {
		http.Error(w, message, http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

func (a *API) signOIDCLoginState(state oidcLoginState) (string, error) {
	payload, err := json.Marshal(state)
	if err != nil {
		return "", fmt.Errorf("marshal oauth state: %w", err)
	}

	payloadPart := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, a.oidcLoginStateSecret())
	_, _ = mac.Write([]byte(payloadPart))
	signaturePart := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return payloadPart + "." + signaturePart, nil
}

func (a *API) parseOIDCLoginState(raw string) (oidcLoginState, error) {
	parts := strings.Split(strings.TrimSpace(raw), ".")
	if len(parts) != 2 {
		return oidcLoginState{}, errors.New("invalid oauth state format")
	}

	mac := hmac.New(sha256.New, a.oidcLoginStateSecret())
	_, _ = mac.Write([]byte(parts[0]))
	expectedSignature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return oidcLoginState{}, errors.New("invalid oauth state signature")
	}
	if !hmac.Equal(mac.Sum(nil), expectedSignature) {
		return oidcLoginState{}, errors.New("oauth state signature mismatch")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return oidcLoginState{}, errors.New("invalid oauth state payload")
	}

	var state oidcLoginState
	if err := json.Unmarshal(payload, &state); err != nil {
		return oidcLoginState{}, errors.New("invalid oauth state body")
	}
	if state.Version != 1 {
		return oidcLoginState{}, errors.New("unsupported oauth state version")
	}
	if state.ReturnTo == "" || state.Nonce == "" || state.PKCEVerifier == "" {
		return oidcLoginState{}, errors.New("oauth state is incomplete")
	}
	if time.Now().UTC().Unix() > state.ExpiresAtUnix {
		return oidcLoginState{}, errors.New("oauth state expired")
	}
	if _, err := a.normalizeOAuthReturnURL(state.ReturnTo); err != nil {
		return oidcLoginState{}, err
	}
	return state, nil
}

func (a *API) oidcLoginStateSecret() []byte {
	sum := sha256.Sum256([]byte(a.config.EncryptionKey + ":oidc-login-state"))
	return sum[:]
}

func randomURLSafeString(byteLen int) (string, error) {
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
