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
	limiter  *rate.Limiter
	lastSeen time.Time
}

// Limiter applies per-IP token-bucket rate limiting and evicts idle visitor state to bound memory.
type Limiter struct {
	rate       rate.Limit
	burst      int
	visitorTTL time.Duration
	pruneEvery time.Duration
	mu         sync.Mutex
	visitors   map[string]*visitorEntry
	lastPrune  time.Time
}

func NewLimiter(perMinute int) *Limiter {
	return newLimiter(perMinute, 15*time.Minute, time.Minute)
}

func newLimiter(perMinute int, visitorTTL, pruneEvery time.Duration) *Limiter {
	if perMinute <= 0 {
		perMinute = 60
	}
	if visitorTTL <= 0 {
		visitorTTL = 15 * time.Minute
	}
	if pruneEvery <= 0 {
		pruneEvery = time.Minute
	}
	return &Limiter{
		rate:       rate.Every(time.Minute / time.Duration(perMinute)),
		burst:      perMinute,
		visitorTTL: visitorTTL,
		pruneEvery: pruneEvery,
		visitors:   make(map[string]*visitorEntry),
	}
}

func (l *Limiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
				limiter:  rate.NewLimiter(l.rate, l.burst),
				lastSeen: now,
			}
			l.visitors[ip] = entry
		}
		entry.lastSeen = now
		lim := entry.limiter
		l.mu.Unlock()

		if !lim.Allow() {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
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
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
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
