package sqlite_test

import (
	"context"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

func TestSQLite_ListUserAPITokens_purgesExpiredWebSessions(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	u, err := s.UpsertOIDCUser(ctx, "tok-user", "tok@x", "Tok")
	if err != nil {
		t.Fatal(err)
	}

	past := time.Now().UTC().Add(-2 * time.Hour)
	if _, _, err := s.CreateUserAPIToken(ctx, u.ID, domain.WebSessionAPITokenName, &past, "", nil, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CreateSessionAPIToken(ctx, u.ID, time.Hour); err != nil {
		t.Fatal(err)
	}
	future := time.Now().UTC().Add(24 * time.Hour)
	if _, _, err := s.CreateUserAPIToken(ctx, u.ID, "expired-ci", &past, "", nil, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CreateUserAPIToken(ctx, u.ID, "active-ci", &future, "", nil, ""); err != nil {
		t.Fatal(err)
	}

	list, err := s.ListUserAPITokens(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}

	byName := map[string]domain.APIToken{}
	for _, tok := range list {
		byName[tok.Name] = tok
	}
	if _, ok := byName[domain.WebSessionAPITokenName]; !ok {
		t.Fatal("expected active web session in list")
	}
	if len(byName) != 3 {
		t.Fatalf("expected 3 tokens (active session, active-ci, expired-ci), got %d: %#v", len(byName), byName)
	}
	if _, ok := byName["expired-ci"]; !ok {
		t.Fatal("expected expired user token to remain listed")
	}
	if tok, ok := byName["expired-ci"]; !ok || !domain.APITokenExpired(tok.ExpiresAt, time.Now().UTC()) {
		t.Fatalf("expired-ci token: %#v", tok)
	}

	var webSessions int
	for _, tok := range list {
		if tok.Name == domain.WebSessionAPITokenName {
			webSessions++
		}
	}
	if webSessions != 1 {
		t.Fatalf("expected exactly one web session token, got %d", webSessions)
	}
}
