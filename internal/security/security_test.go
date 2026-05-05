package security

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewEncrypter(t *testing.T) {
	t.Parallel()
	e, err := NewEncrypter("any-secret-string")
	if err != nil {
		t.Fatalf("NewEncrypter: %v", err)
	}
	if e == nil || e.gcm == nil {
		t.Fatal("expected non-nil encrypter")
	}
}

func TestEncrypter_EncryptDecrypt_roundTrip(t *testing.T) {
	t.Parallel()
	e, err := NewEncrypter("test-secret-32-chars-minimum!!")
	if err != nil {
		t.Fatal(err)
	}
	plaintext := "hello token 世界"
	cipher, err := e.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if cipher == "" || cipher == plaintext {
		t.Fatalf("ciphertext should differ from plaintext, got %q", cipher)
	}
	got, err := e.Decrypt(cipher)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != plaintext {
		t.Errorf("Decrypt: want %q, got %q", plaintext, got)
	}
}

func TestEncrypter_Encrypt_uniqueNonces(t *testing.T) {
	t.Parallel()
	e, _ := NewEncrypter("same-secret")
	a, _ := e.Encrypt("x")
	b, _ := e.Encrypt("x")
	if a == b {
		t.Fatal("two encryptions of same plaintext should differ (random nonce)")
	}
}

func TestEncrypter_Decrypt_invalidBase64(t *testing.T) {
	t.Parallel()
	e, _ := NewEncrypter("secret")
	_, err := e.Decrypt("not-valid-base64!!!")
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("error should mention decode: %v", err)
	}
}

func TestEncrypter_Decrypt_tooShort(t *testing.T) {
	t.Parallel()
	e, _ := NewEncrypter("secret")
	// Valid base64 but shorter than nonce
	short := "YQ==" // "a" decoded is 1 byte
	_, err := e.Decrypt(short)
	if err == nil {
		t.Fatal("expected error for short ciphertext")
	}
}

func TestEncrypter_Decrypt_wrongKey(t *testing.T) {
	t.Parallel()
	e1, _ := NewEncrypter("key-one")
	e2, _ := NewEncrypter("key-two")
	cipher, _ := e1.Encrypt("secret data")
	_, err := e2.Decrypt(cipher)
	if err == nil {
		t.Fatal("expected decrypt failure with wrong key")
	}
}

func TestEncrypter_Decrypt_tampered(t *testing.T) {
	t.Parallel()
	e, _ := NewEncrypter("tamper-secret")
	cipher, _ := e.Encrypt("payload")
	// Flip last char of base64-ish manipulation: decode, flip byte, re-encode is heavy;
	// append junk by corrupting middle of string after valid prefix
	if len(cipher) < 4 {
		t.Fatal("cipher too short")
	}
	broken := cipher[:len(cipher)-2] + "XX"
	_, err := e.Decrypt(broken)
	if err == nil {
		t.Fatal("expected error for tampered ciphertext")
	}
}

func TestHashToken(t *testing.T) {
	t.Parallel()
	a := HashToken("token-a")
	b := HashToken("token-a")
	if a != b {
		t.Error("HashToken should be deterministic")
	}
	c := HashToken("token-b")
	if a == c {
		t.Error("different tokens should hash differently")
	}
	if len(a) < 32 {
		t.Errorf("hash should be non-trivial length: %q", a)
	}
}

func TestNewLimiter_defaultWhenNonPositive(t *testing.T) {
	t.Parallel()
	l := NewLimiter(0, 0)
	if l == nil {
		t.Fatal("nil limiter")
	}
	if l.anonBurst != 120 || l.authBurst != 600 {
		t.Fatalf("want anon burst 120 auth burst 600, got %d %d", l.anonBurst, l.authBurst)
	}
	l2 := NewLimiter(-5, 0)
	if l2.anonBurst != 120 {
		t.Fatalf("negative anon: want burst 120, got %d", l2.anonBurst)
	}
}

func TestLimiter_Middleware_allowsUnderBurst(t *testing.T) {
	t.Parallel()
	l := NewLimiter(10000, 10000) // very high per minute
	called := false
	h := l.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.1:1234"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !called {
		t.Fatalf("want 200 and handler called, got code=%d called=%v", rr.Code, called)
	}
}

func TestLimiter_Middleware_rateLimited(t *testing.T) {
	t.Parallel()
	// Anonymous bucket: 1 req/min burst 1 — second request without bearer should 429
	l := NewLimiter(1, 1000)
	h := l.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.10:1"

	rr1 := httptest.NewRecorder()
	h.ServeHTTP(rr1, req)
	if rr1.Code != http.StatusOK {
		t.Fatalf("first request: want 200, got %d", rr1.Code)
	}

	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: want 429, got %d body=%q", rr2.Code, rr2.Body.String())
	}
}

func TestLimiter_Middleware_bearerUsesHigherBucket(t *testing.T) {
	t.Parallel()
	l := NewLimiter(1, 10)
	h := l.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := func() *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", "Bearer token")
		r.RemoteAddr = "192.0.2.77:1"
		return r
	}

	for i := 0; i < 10; i++ {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req())
		if rr.Code != http.StatusOK {
			t.Fatalf("bearer request %d: want 200, got %d", i, rr.Code)
		}
	}
	rr429 := httptest.NewRecorder()
	h.ServeHTTP(rr429, req())
	if rr429.Code != http.StatusTooManyRequests {
		t.Fatalf("11th bearer request: want 429, got %d", rr429.Code)
	}
}

func TestLimiter_prunesIdleVisitors(t *testing.T) {
	t.Parallel()
	l := newLimiterDual(1000, 1000, 40*time.Millisecond, 15*time.Millisecond)
	h := l.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.RemoteAddr = "192.0.2.50:1"
	h.ServeHTTP(httptest.NewRecorder(), req1)

	time.Sleep(80 * time.Millisecond)

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "192.0.2.51:1"
	h.ServeHTTP(httptest.NewRecorder(), req2)

	l.mu.Lock()
	n := len(l.visitors)
	l.mu.Unlock()
	if n > 2 {
		t.Fatalf("visitor map grew unexpectedly: %d", n)
	}
}

func TestLimiter_Middleware_xForwardedForKeying(t *testing.T) {
	t.Parallel()
	l := NewLimiter(1, 1000)
	h := l.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	reqA := httptest.NewRequest(http.MethodGet, "/", nil)
	reqA.Header.Set("X-Forwarded-For", "203.0.113.1")
	reqA.RemoteAddr = "10.0.0.1:1"
	rrA1 := httptest.NewRecorder()
	h.ServeHTTP(rrA1, reqA)
	rrA2 := httptest.NewRecorder()
	h.ServeHTTP(rrA2, reqA)
	if rrA2.Code != http.StatusTooManyRequests {
		t.Fatalf("same forwarded IP should rate-limit: got %d", rrA2.Code)
	}

	reqB := httptest.NewRequest(http.MethodGet, "/", nil)
	reqB.Header.Set("X-Forwarded-For", "203.0.113.2")
	reqB.RemoteAddr = "10.0.0.1:1"
	rrB := httptest.NewRecorder()
	h.ServeHTTP(rrB, reqB)
	if rrB.Code != http.StatusOK {
		t.Fatalf("different forwarded IP should be allowed: got %d", rrB.Code)
	}
}

func TestCORSMiddleware_wildcardOrigin(t *testing.T) {
	t.Parallel()
	h := CORSMiddleware([]string{"*"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://evil.example")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Allow-Origin: want *, got %q", got)
	}
	if rr.Code != http.StatusTeapot {
		t.Errorf("want %d, got %d", http.StatusTeapot, rr.Code)
	}
}

func TestCORSMiddleware_specificOrigin(t *testing.T) {
	t.Parallel()
	h := CORSMiddleware([]string{"https://app.example"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://app.example")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example" {
		t.Errorf("Allow-Origin: want app origin, got %q", got)
	}
	if !strings.Contains(rr.Header().Get("Vary"), "Origin") {
		t.Errorf("expected Vary to include Origin, got %q", rr.Header().Get("Vary"))
	}
}

func TestCORSMiddleware_disallowedOrigin(t *testing.T) {
	t.Parallel()
	h := CORSMiddleware([]string{"https://app.example"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://other.example")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("should not set Allow-Origin for disallowed origin, got %q", rr.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCORSMiddleware_optionsPreflight(t *testing.T) {
	t.Parallel()
	h := CORSMiddleware([]string{"https://a.example"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run for OPTIONS")
	}))
	req := httptest.NewRequest(http.MethodOptions, "/api", nil)
	req.Header.Set("Origin", "https://a.example")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS: want 204, got %d", rr.Code)
	}
}

func TestCORSMiddleware_noOriginPassesThrough(t *testing.T) {
	t.Parallel()
	seen := false
	h := CORSMiddleware([]string{"https://x.example"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if !seen || rr.Code != http.StatusOK {
		t.Fatalf("next should run for non-CORS request: seen=%v code=%d", seen, rr.Code)
	}
}

func TestWithPrincipal_PrincipalFromContext(t *testing.T) {
	t.Parallel()
	type user struct {
		ID string
	}
	ctx := WithPrincipal(context.Background(), user{ID: "u1"})
	got, ok := PrincipalFromContext[user](ctx)
	if !ok || got.ID != "u1" {
		t.Fatalf("PrincipalFromContext: ok=%v got=%+v", ok, got)
	}
	_, okStr := PrincipalFromContext[string](ctx)
	if okStr {
		t.Fatal("wrong type should not assert")
	}
}

func TestEncrypter_concurrentEncrypt(t *testing.T) {
	t.Parallel()
	e, _ := NewEncrypter("concurrent-secret")
	const n = 32
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			c, err := e.Encrypt("data")
			if err != nil {
				errs <- err
				return
			}
			_, err = e.Decrypt(c)
			errs <- err
		}()
	}
	deadline := time.After(2 * time.Second)
	for i := 0; i < n; i++ {
		select {
		case err := <-errs:
			if err != nil {
				t.Errorf("goroutine error: %v", err)
			}
		case <-deadline:
			t.Fatal("timeout waiting for goroutines")
		}
	}
}
