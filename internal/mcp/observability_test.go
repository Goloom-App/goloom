package mcp

import (
	"context"
	"testing"

	"git.f4mily.net/goloom/internal/agenttools"
	"git.f4mily.net/goloom/internal/domain"
)

// mcpActor is tested directly (same package) since it is unexported.

func TestMcpActor_NoPrincipalReturnsUnknown(t *testing.T) {
	ctx := context.Background()
	if got := mcpActor(ctx); got != "unknown" {
		t.Fatalf("mcpActor without principal = %q, want 'unknown'", got)
	}
}

func TestMcpActor_WithTokenNamePreferredOverEmail(t *testing.T) {
	name := "my-api-token"
	principal := domain.AuthenticatedPrincipal{
		User:      domain.User{ID: "u-1", Email: "user@example.com"},
		TokenName: &name,
	}
	ctx := agenttools.WithPrincipal(context.Background(), principal)
	if got := mcpActor(ctx); got != "my-api-token" {
		t.Fatalf("mcpActor with token name = %q, want 'my-api-token'", got)
	}
}

func TestMcpActor_BlankTokenNameFallsBackToEmail(t *testing.T) {
	blank := "   "
	principal := domain.AuthenticatedPrincipal{
		User:      domain.User{ID: "u-2", Email: "editor@example.com"},
		TokenName: &blank,
	}
	ctx := agenttools.WithPrincipal(context.Background(), principal)
	if got := mcpActor(ctx); got != "editor@example.com" {
		t.Fatalf("mcpActor with blank token name = %q, want email", got)
	}
}

func TestMcpActor_WithEmailReturnsEmail(t *testing.T) {
	principal := domain.AuthenticatedPrincipal{
		User: domain.User{ID: "u-3", Email: "person@example.com"},
	}
	ctx := agenttools.WithPrincipal(context.Background(), principal)
	if got := mcpActor(ctx); got != "person@example.com" {
		t.Fatalf("mcpActor with email = %q, want 'person@example.com'", got)
	}
}

func TestMcpActor_NoEmailFallsBackToUserID(t *testing.T) {
	principal := domain.AuthenticatedPrincipal{
		User: domain.User{ID: "u-fallback", Email: ""},
	}
	ctx := agenttools.WithPrincipal(context.Background(), principal)
	if got := mcpActor(ctx); got != "u-fallback" {
		t.Fatalf("mcpActor without email = %q, want 'u-fallback'", got)
	}
}

func TestMcpActor_NilTokenNameFallsBackToEmail(t *testing.T) {
	principal := domain.AuthenticatedPrincipal{
		User:      domain.User{ID: "u-5", Email: "user5@example.com"},
		TokenName: nil,
	}
	ctx := agenttools.WithPrincipal(context.Background(), principal)
	if got := mcpActor(ctx); got != "user5@example.com" {
		t.Fatalf("mcpActor with nil token name = %q, want email", got)
	}
}
