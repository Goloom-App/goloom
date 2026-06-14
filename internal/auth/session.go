package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
)

// Web-session cookies. The session cookie is the bearer-equivalent for browser
// sessions; the CSRF cookie is a readable double-submit token. API tokens (MCP,
// tools) use Authorization: Bearer and never touch these.
const (
	SessionCookieName = "goloom_session"
	CSRFCookieName    = "goloom_csrf"
	CSRFHeaderName    = "X-CSRF-Token"
)

// NewCSRFToken returns a random URL-safe token for the double-submit cookie.
func NewCSRFToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// WriteSessionCookies sets (or rolls) the HttpOnly session cookie and the
// readable CSRF cookie with a fresh Max-Age.
func (s *Service) WriteSessionCookies(w http.ResponseWriter, sessionToken, csrfToken string) {
	maxAge := int(s.sessionTTL.Seconds())
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    sessionToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    csrfToken,
		Path:     "/",
		HttpOnly: false,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	})
}

// ClearSessionCookies expires both cookies (used on sign-out).
func (s *Service) ClearSessionCookies(w http.ResponseWriter) {
	for _, name := range []string{SessionCookieName, CSRFCookieName} {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			HttpOnly: name == SessionCookieName,
			Secure:   s.secureCookies,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
		})
	}
}

func isUnsafeMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func cookieValue(r *http.Request, name string) string {
	if c, err := r.Cookie(name); err == nil {
		return c.Value
	}
	return ""
}

// csrfValid reports whether the request carries a matching double-submit token.
func csrfValid(r *http.Request) bool {
	cookie := cookieValue(r, CSRFCookieName)
	header := r.Header.Get(CSRFHeaderName)
	if cookie == "" || header == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookie), []byte(header)) == 1
}
