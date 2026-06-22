package security

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type Encrypter struct {
	gcm cipher.AEAD
}

func NewEncrypter(secret string) (*Encrypter, error) {
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}
	return &Encrypter{gcm: gcm}, nil
}

func (e *Encrypter) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("read nonce: %w", err)
	}
	ciphertext := e.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (e *Encrypter) Decrypt(ciphertext string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decode token: %w", err)
	}
	nonceSize := e.gcm.NonceSize()
	if len(raw) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, encrypted := raw[:nonceSize], raw[nonceSize:]
	plaintext, err := e.gcm.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt token: %w", err)
	}
	return string(plaintext), nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.StdEncoding.EncodeToString(sum[:])
}

type visitorEntry struct {
	anon     *rate.Limiter
	auth     *rate.Limiter
	lastSeen time.Time
}

// Limiter applies per-IP token-bucket rate limiting and evicts idle visitor state to bound memory.
// Requests without Authorization: Bearer use anonymousPerMinute; requests with a bearer token use
// authenticatedPerMinute (intended for SPA and API clients after login).
type Limiter struct {
	anonRate   rate.Limit
	anonBurst  int
	authRate   rate.Limit
	authBurst  int
	visitorTTL time.Duration
	pruneEvery time.Duration
	mu         sync.Mutex
	visitors   map[string]*visitorEntry
	lastPrune  time.Time
}

// NewLimiter builds a limiter. Non-positive anonymousPerMinute defaults to 120/min.
// Non-positive authenticatedPerMinute defaults to max(300, 5*anonymousPerMinute).
// If authenticatedPerMinute is lower than anonymousPerMinute, it is raised to match.
func NewLimiter(anonymousPerMinute, authenticatedPerMinute int) *Limiter {
	if anonymousPerMinute <= 0 {
		anonymousPerMinute = 120
	}
	if authenticatedPerMinute <= 0 {
		authenticatedPerMinute = anonymousPerMinute * 5
		if authenticatedPerMinute < 300 {
			authenticatedPerMinute = 300
		}
	}
	if authenticatedPerMinute < anonymousPerMinute {
		authenticatedPerMinute = anonymousPerMinute
	}
	return newLimiterDual(anonymousPerMinute, authenticatedPerMinute, 15*time.Minute, time.Minute)
}

func newLimiterDual(anonPerMinute, authPerMinute int, visitorTTL, pruneEvery time.Duration) *Limiter {
	if visitorTTL <= 0 {
		visitorTTL = 15 * time.Minute
	}
	if pruneEvery <= 0 {
		pruneEvery = time.Minute
	}
	return &Limiter{
		anonRate:   rate.Every(time.Minute / time.Duration(anonPerMinute)),
		anonBurst:  anonPerMinute,
		authRate:   rate.Every(time.Minute / time.Duration(authPerMinute)),
		authBurst:  authPerMinute,
		visitorTTL: visitorTTL,
		pruneEvery: pruneEvery,
		visitors:   make(map[string]*visitorEntry),
	}
}

// Middleware rate-limits before invoking next. GET /healthz is not counted (liveness probes).
func (l *Limiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}

		ip := clientIP(r)
		now := time.Now()

		l.mu.Lock()
		if now.Sub(l.lastPrune) >= l.pruneEvery {
			cutoff := now.Add(-l.visitorTTL)
			for key, v := range l.visitors {
				if v.lastSeen.Before(cutoff) {
					delete(l.visitors, key)
				}
			}
			l.lastPrune = now
		}
		entry := l.visitors[ip]
		if entry == nil {
			entry = &visitorEntry{
				anon:     rate.NewLimiter(l.anonRate, l.anonBurst),
				auth:     rate.NewLimiter(l.authRate, l.authBurst),
				lastSeen: now,
			}
			l.visitors[ip] = entry
		}
		entry.lastSeen = now
		lim := entry.anon
		if requestHasBearer(r) {
			lim = entry.auth
		}
		l.mu.Unlock()

		if !lim.Allow() {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func requestHasBearer(r *http.Request) bool {
	s := strings.TrimSpace(r.Header.Get("Authorization"))
	return len(s) > 7 && strings.EqualFold(s[:7], "bearer ")
}

func CORSMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[origin] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				if _, ok := allowed["*"]; ok {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else if _, ok := allowed[origin]; ok {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Add("Vary", "Origin")
				}
				// Mcp-Session-Id / Mcp-Protocol-Version are the MCP Streamable HTTP
				// session headers: browser clients must be allowed to send them on
				// follow-up requests and to read Mcp-Session-Id back from the
				// initialize response, or the session is lost after handshake.
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept, Mcp-Session-Id, Mcp-Protocol-Version")
				w.Header().Set("Access-Control-Expose-Headers", "Mcp-Session-Id, Mcp-Protocol-Version")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

type principalKey struct{}

func WithPrincipal(ctx context.Context, value any) context.Context {
	return context.WithValue(ctx, principalKey{}, value)
}

func PrincipalFromContext[T any](ctx context.Context) (T, bool) {
	value, ok := ctx.Value(principalKey{}).(T)
	return value, ok
}

func clientIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); forwarded != "" {
		return forwarded
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
