package agenttools

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/postservice"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/store/sqlite"
	"github.com/google/uuid"
)

// fixture wires a real sqlite store, auth service, provider registry and the
// agenttools Deps so the catalog's core functions can be exercised exactly as
// the MCP and chat adapters call them.
type fixture struct {
	store   *sqlite.Store
	deps    Deps
	user    domain.User
	team    domain.Team
	account domain.SocialAccount
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	ctx := context.Background()
	enc, err := security.NewEncrypter("agenttools-test-secret-32-bytes!")
	if err != nil {
		t.Fatal(err)
	}
	s, err := sqlite.New(ctx, "file:"+uuid.NewString()+"?mode=memory&cache=shared", enc)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	authSvc, err := auth.New(ctx, config.Config{}, s)
	if err != nil {
		t.Fatal(err)
	}
	// Burn the first-user-is-admin slot so test users are regular users.
	if _, err := s.UpsertOIDCUser(ctx, "seed-admin", "seed@at.test", "Seed"); err != nil {
		t.Fatal(err)
	}
	u, err := s.UpsertOIDCUser(ctx, "at-"+uuid.NewString(), "at@test", "AT")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "at-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	acc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://m.example", Username: "at", AccessToken: "tok",
	})
	if err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	registry := provider.NewRegistry(
		provider.NewBlueskyProvider(),
		provider.NewMastodonProvider(provider.MastodonRegistrationConfig{}),
	)
	deps := Deps{
		Store:     s,
		Auth:      authSvc,
		Posts:     postservice.New(s, registry),
		Providers: registry,
		Logger:    logger,
		Audit: func(ctx context.Context, event domain.AuditEvent) {
			if err := s.InsertAuditEvent(ctx, event); err != nil {
				t.Errorf("audit insert: %v", err)
			}
		},
	}
	return fixture{store: s, deps: deps, user: u, team: team, account: acc}
}

// principal mints an API token with the given scopes and resolves it to a
// principal, the same value the adapters bind into an Invocation.
func (f fixture) principal(t *testing.T, scopes string) domain.AuthenticatedPrincipal {
	t.Helper()
	token, _, err := f.store.CreateUserAPIToken(context.Background(), f.user.ID, "at-test", nil, scopes, nil, "")
	if err != nil {
		t.Fatalf("CreateUserAPIToken: %v", err)
	}
	p, err := f.store.LookupAPIToken(context.Background(), token)
	if err != nil {
		t.Fatalf("LookupAPIToken: %v", err)
	}
	return p
}

// inv builds an Invocation for the given scopes over the MCP transport.
func (f fixture) inv(t *testing.T, scopes string) Invocation {
	return Invocation{Principal: f.principal(t, scopes), Transport: TransportMCP}
}

// blueskyAccount adds a Bluesky account (300-char limit) to the fixture's team.
func (f fixture) blueskyAccount(t *testing.T) domain.SocialAccount {
	t.Helper()
	acc, err := f.store.CreateAccount(context.Background(), f.team.ID, domain.ConnectedAccount{
		Provider: "bluesky", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://bsky.social", Username: "bsky-" + uuid.NewString(), AccessToken: "tok",
	})
	if err != nil {
		t.Fatalf("CreateAccount bluesky: %v", err)
	}
	return acc
}

// foreignAccount creates a second team (same owner) with its own account, to
// exercise cross-team targeting protection.
func (f fixture) foreignAccount(t *testing.T) domain.SocialAccount {
	t.Helper()
	other, err := f.store.CreateTeam(context.Background(), f.user.ID, domain.CreateTeamInput{Name: "other-" + uuid.NewString()})
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	acc, err := f.store.CreateAccount(context.Background(), other.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://m.example", Username: "foreign", AccessToken: "tok",
	})
	if err != nil {
		t.Fatalf("CreateAccount foreign: %v", err)
	}
	return acc
}

func soon() string { return time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339) }
