package pagefetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/netguard"
)

func TestFetchTextGuardsAndBounds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/large":
			_, _ = w.Write([]byte(strings.Repeat("x", maxBodyBytes+1)))
		case "/slow":
			<-r.Context().Done()
		case "/redirect":
			http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
		case "/binary":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write([]byte("%PDF-1.7"))
		case "/binary-unknown":
			w.Header().Set("Content-Type", "")
			_, _ = w.Write([]byte{0, 1, 2, 3})
		default:
			_, _ = w.Write([]byte("<article>Useful article detail from linked page.</article>"))
		}
	}))
	defer server.Close()
	policy := netguard.NewPolicy([]string{strings.TrimPrefix(server.URL, "http://")})
	for _, raw := range []string{"http://127.0.0.2/", "http://169.254.169.254/", server.URL + "/redirect", "http://user:pass@127.0.0.1/"} {
		if _, err := FetchText(context.Background(), raw, policy, time.Second); err == nil {
			t.Errorf("unsafe URL %q accepted", raw)
		}
	}
	if _, err := FetchText(context.Background(), server.URL+"/large", policy, time.Second); err == nil {
		t.Fatal("oversize page accepted")
	}
	if _, err := FetchText(context.Background(), server.URL+"/binary", policy, time.Second); err == nil {
		t.Fatal("binary page accepted")
	}
	if _, err := FetchText(context.Background(), server.URL+"/binary-unknown", policy, time.Second); err == nil {
		t.Fatal("untyped binary page accepted")
	}
	if _, err := FetchText(context.Background(), server.URL+"/slow", policy, 20*time.Millisecond); err == nil {
		t.Fatal("timed out page accepted")
	}
	got, err := FetchText(context.Background(), server.URL, policy, time.Second)
	if err != nil || !strings.Contains(got, "Useful article detail") {
		t.Fatalf("readable article = %q, %v", got, err)
	}
}
