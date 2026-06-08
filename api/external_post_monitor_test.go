package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/i18n"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/security"
	sqlitestore "git.f4mily.net/goloom/internal/store/sqlite"
)

func TestExternalPostMonitor_ownerCanEnable(t *testing.T) {
	ctx := context.Background()
	s := newValidationMemoryStore(t)
	owner, err := s.UpsertOIDCUser(ctx, "epm-owner", "epm-owner@example.com", "Owner")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, owner.ID, domain.CreateTeamInput{Name: "epm-team", Description: ""})
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := s.CreateSessionAPIToken(ctx, owner.ID, 0)
	if err != nil {
		t.Fatal(err)
	}

	h := newExternalPostMonitorTestHandler(t, s)
	body, _ := json.Marshal(map[string]bool{"enabled": true})
	req := httptest.NewRequest(http.MethodPut, "/v1/teams/"+team.ID+"/external-post-monitor", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	var out domain.ExternalPostMonitorSettings
	if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if !out.Enabled {
		t.Fatal("expected enabled")
	}
}

func TestExternalPostMonitor_viewerCannotEnable(t *testing.T) {
	ctx := context.Background()
	s := newValidationMemoryStore(t)
	owner, _ := s.UpsertOIDCUser(ctx, "epm-owner2", "owner2@example.com", "Owner")
	viewer, _ := s.UpsertOIDCUser(ctx, "epm-viewer", "viewer@example.com", "Viewer")
	team, _ := s.CreateTeam(ctx, owner.ID, domain.CreateTeamInput{Name: "epm-team2", Description: ""})
	_, _ = s.AddTeamMember(ctx, team.ID, domain.AddTeamMemberInput{UserID: viewer.ID, Role: domain.RoleViewer})
	token, _, _ := s.CreateSessionAPIToken(ctx, viewer.ID, 0)

	h := newExternalPostMonitorTestHandler(t, s)
	body, _ := json.Marshal(map[string]bool{"enabled": true})
	req := httptest.NewRequest(http.MethodPut, "/v1/teams/"+team.ID+"/external-post-monitor", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body %s", rr.Code, rr.Body.String())
	}
}

func newExternalPostMonitorTestHandler(t *testing.T, s *sqlitestore.Store) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	authSvc, err := auth.New(context.Background(), config.Config{}, s)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	reg := provider.NewRegistry(provider.NewMastodonProvider(provider.MastodonRegistrationConfig{}))
	api := New(logger, s, authSvc, reg, config.Config{}, nil, catalog, nil, nil)
	return api.Handler(security.NewLimiter(1000, 1000), nil)
}
