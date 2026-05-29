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
	expectedScopes := []string{"ai:read:context", "ai:write:drafts"}
	plaintext, _, err := s.CreateUserAPIToken(ctx, user.ID, "scoped", nil, `["ai:read:context","ai:write:drafts"]`, &team.ID)
	if err != nil {
		t.Fatal(err)
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
}
