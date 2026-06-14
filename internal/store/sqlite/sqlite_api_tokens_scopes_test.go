package sqlite_test

import (
	"context"
	"reflect"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/google/uuid"
)

func TestAPITokenScopes(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	user, err := s.UpsertOIDCUser(ctx, "tok-scope-"+uuid.NewString(), "tok-scope@example.test", "Tok Scope")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, user.ID, domain.CreateTeamInput{Name: "scope-team", Description: "scope team"})
	if err != nil {
		t.Fatal(err)
	}
	expectedScopes := []string{"read", "write:draft"}
	plaintext, meta, err := s.CreateUserAPIToken(ctx, user.ID, "scoped", nil, `["read","write:draft"]`, &team.ID, "CI bot")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Description != "CI bot" || !reflect.DeepEqual(meta.Scopes, expectedScopes) {
		t.Fatalf("create meta=%#v", meta)
	}

	principal, err := s.LookupAPIToken(ctx, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(principal.Scopes, expectedScopes) {
		t.Fatalf("scopes=%#v want=%#v", principal.Scopes, expectedScopes)
	}
	if principal.TokenTeamID == nil || *principal.TokenTeamID != team.ID {
		t.Fatalf("token team=%v want=%s", principal.TokenTeamID, team.ID)
	}

	// Description round-trips through the listing.
	tokens, err := s.ListUserAPITokens(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, tk := range tokens {
		if tk.Name == "scoped" {
			found = true
			if tk.Description != "CI bot" || !reflect.DeepEqual(tk.Scopes, expectedScopes) {
				t.Fatalf("listed token=%#v", tk)
			}
		}
	}
	if !found {
		t.Fatal("created token not listed")
	}
}
