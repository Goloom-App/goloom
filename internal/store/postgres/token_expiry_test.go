package postgres_test

import (
	"context"
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestLookupAPITokenCarriesExpiry(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	bearer := "expiry-bootstrap-" + uuid.NewString()
	if err := s.EnsureBootstrapAdmin(ctx, "expiry@example.test", "Expiry", bearer); err != nil {
		t.Fatal(err)
	}
	principal, err := s.LookupAPIToken(ctx, bearer)
	if err != nil {
		t.Fatal(err)
	}
	if principal.TokenExpiresAt != nil {
		t.Fatalf("bootstrap expiry = %v, want nil", principal.TokenExpiresAt)
	}
	expires := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	token, _, err := s.CreateUserAPIToken(ctx, principal.User.ID, "expiring", &expires, `["read"]`, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	principal, err = s.LookupAPIToken(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if principal.TokenExpiresAt == nil || !principal.TokenExpiresAt.Equal(expires) {
		t.Fatalf("principal expiry = %v, want %v", principal.TokenExpiresAt, expires)
	}
}
