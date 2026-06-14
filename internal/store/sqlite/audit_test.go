package sqlite_test

import (
	"context"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

func TestSQLite_AuditEvents_InsertListFilter(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	user, err := s.UpsertOIDCUser(ctx, "audit-sub", "owner@example.test", "Owner")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, user.ID, domain.CreateTeamInput{Name: "Audit Team", Description: "d"})
	if err != nil {
		t.Fatal(err)
	}

	tokenName := "ci-bot"
	tokenID := "tok-1"
	postID := "post-1"
	seed := []domain.AuditEvent{
		{TeamID: team.ID, ActorUserID: user.ID, ActorName: "Owner", ActorKind: domain.AuditActorHuman, Action: "post.create", TargetType: "post", TargetID: &postID, Summary: "Created post: Hello"},
		{TeamID: team.ID, ActorUserID: user.ID, ActorName: "Owner", ActorKind: domain.AuditActorHuman, Action: "post.delete", TargetType: "post", TargetID: &postID},
		{TeamID: team.ID, ActorUserID: user.ID, ActorName: "Owner", ActorKind: domain.AuditActorToken, TokenID: &tokenID, TokenName: &tokenName, Action: "post.create", TargetType: "post", Metadata: map[string]string{"k": "v"}},
	}
	for _, e := range seed {
		if err := s.InsertAuditEvent(ctx, e); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	// Filter by action.
	created, err := s.ListAuditEvents(ctx, domain.AuditFilter{TeamID: team.ID, Action: "post.create"})
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 2 {
		t.Fatalf("post.create filter returned %d, want 2", len(created))
	}

	// Token attribution + metadata round-trips.
	var tokenEvent *domain.AuditEvent
	for i := range created {
		if created[i].ActorKind == domain.AuditActorToken {
			tokenEvent = &created[i]
		}
	}
	if tokenEvent == nil {
		t.Fatal("expected a token-attributed event")
	}
	if tokenEvent.TokenName == nil || *tokenEvent.TokenName != tokenName {
		t.Fatalf("token name = %v, want %q", tokenEvent.TokenName, tokenName)
	}
	if tokenEvent.Metadata["k"] != "v" {
		t.Fatalf("metadata = %v, want k=v", tokenEvent.Metadata)
	}

	// Count of all events for the team.
	total, err := s.CountAuditEvents(ctx, domain.AuditFilter{TeamID: team.ID})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 {
		t.Fatalf("count = %d, want 3", total)
	}

	// Unknown team returns an empty (non-nil) slice.
	none, err := s.ListAuditEvents(ctx, domain.AuditFilter{TeamID: "no-such-team"})
	if err != nil {
		t.Fatal(err)
	}
	if none == nil || len(none) != 0 {
		t.Fatalf("unknown team should return empty slice, got %v", none)
	}
}

func TestSQLite_LookupAPIToken_Attribution(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	user, err := s.UpsertOIDCUser(ctx, "attr-sub", "attr@example.test", "Attr")
	if err != nil {
		t.Fatal(err)
	}

	// A named API key is attributed to the token (tool).
	plain, meta, err := s.CreateUserAPIToken(ctx, user.ID, "mybot", nil, "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	principal, err := s.LookupAPIToken(ctx, plain)
	if err != nil {
		t.Fatal(err)
	}
	if principal.Kind != domain.AuditActorToken {
		t.Fatalf("kind = %q, want api_token", principal.Kind)
	}
	if principal.TokenID == nil || *principal.TokenID != meta.ID {
		t.Fatalf("token id = %v, want %q", principal.TokenID, meta.ID)
	}
	if principal.TokenName == nil || *principal.TokenName != "mybot" {
		t.Fatalf("token name = %v, want 'mybot'", principal.TokenName)
	}

	// A web session is a human; it carries no token attribution.
	sessionPlain, _, err := s.CreateSessionAPIToken(ctx, user.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	sessionPrincipal, err := s.LookupAPIToken(ctx, sessionPlain)
	if err != nil {
		t.Fatal(err)
	}
	if sessionPrincipal.Kind != domain.AuditActorHuman {
		t.Fatalf("web session kind = %q, want oidc", sessionPrincipal.Kind)
	}
	if sessionPrincipal.TokenID != nil || sessionPrincipal.TokenName != nil {
		t.Fatalf("web session must not be token-attributed: id=%v name=%v", sessionPrincipal.TokenID, sessionPrincipal.TokenName)
	}
}
