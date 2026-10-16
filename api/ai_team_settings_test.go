package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"git.f4mily.net/goloom/api"
	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/i18n"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/store"
	"github.com/google/uuid"
	"net/http/httptest"
)

// failingProfileStore makes GetTeamProfile fail with a non-not-found error and
// records whether a create was attempted.
type failingProfileStore struct {
	store.Store
	created bool
}

func (f *failingProfileStore) GetTeamProfile(context.Context, string) (domain.TeamProfile, error) {
	return domain.TeamProfile{}, errors.New("connection reset by peer")
}

func (f *failingProfileStore) CreateTeamProfile(ctx context.Context, teamID string, in domain.TeamProfile) (domain.TeamProfile, error) {
	f.created = true
	return f.Store.CreateTeamProfile(ctx, teamID, in)
}

func TestUpsertTeamProfile_StoreErrorIsNotTreatedAsNotFound(t *testing.T) {
	f := seedAICRUDTeam(t, true)
	wrapped := &failingProfileStore{Store: f.store}
	ctx := context.Background()
	authSvc, err := auth.New(ctx, config.Config{}, wrapped)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	reg := provider.NewRegistry(provider.NewBlueskyProvider())
	h := api.New(nil, wrapped, authSvc, reg, config.Config{}, nil, catalog, nil, nil).
		Handler(security.NewLimiter(10_000, 10_000), nil)

	rec := doRequest(t, h, http.MethodPut, "/v1/teams/"+f.teamID+"/profile", f.bearer, map[string]any{
		"style_metadata": map[string]any{"tonality": "x"},
	})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d: %s", rec.Code, rec.Body.String())
	}
	if wrapped.created {
		t.Fatal("profile must not be created when the lookup failed for a non-not-found reason")
	}
	if strings.Contains(rec.Body.String(), "connection reset") {
		t.Fatalf("raw store error leaked to client: %s", rec.Body.String())
	}
	if got := rec.Header().Get("X-Error-Code"); got != "internal_error" {
		t.Fatalf("X-Error-Code: got %q", got)
	}
}

func assertErr(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status: got %d want %d; body=%q", rec.Code, status, rec.Body.String())
	}
	if got := rec.Header().Get("X-Error-Code"); got != code {
		t.Fatalf("X-Error-Code: got %q want %q (body=%q)", got, code, rec.Body.String())
	}
}

func auditActions(t *testing.T, f aiCRUDFixture) []string {
	t.Helper()
	events, err := f.store.ListAuditEvents(context.Background(), domain.AuditFilter{TeamID: f.teamID, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(events))
	for _, e := range events {
		out = append(out, e.Action)
	}
	return out
}

func hasAction(actions []string, want string) bool {
	for _, a := range actions {
		if a == want {
			return true
		}
	}
	return false
}

func TestTeamProfileValidation(t *testing.T) {
	f := seedAICRUDTeam(t, true)
	path := "/v1/teams/" + f.teamID + "/profile"
	cases := map[string]map[string]any{
		"max_hashtags too high": {"max_hashtags": 31},
		"max_hashtags negative": {"max_hashtags": -1},
		"tonality too long":     {"tonality": strings.Repeat("x", 201)},
		"too many banned words": {"banned_words": make([]string, 201)},
		"too many rules":        {"formatting_rules": make([]string, 51)},
	}
	for name, meta := range cases {
		t.Run(name, func(t *testing.T) {
			rec := doRequest(t, f.h, http.MethodPut, path, f.bearer, map[string]any{"style_metadata": meta})
			assertErr(t, rec, http.StatusBadRequest, "invalid_team_profile")
		})
	}
	rec := doRequest(t, f.h, http.MethodPut, path, f.bearer, map[string]any{"style_metadata": map[string]any{"max_hashtags": 30}})
	if rec.Code != http.StatusOK {
		t.Fatalf("boundary 30 must pass: %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, f.h, http.MethodDelete, path, f.bearer, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rec.Code)
	}
	actions := auditActions(t, f)
	if !hasAction(actions, "brand_profile.update") || !hasAction(actions, "brand_profile.delete") {
		t.Fatalf("missing audit, got %v", actions)
	}
}

func TestCampaignFormatValidationAndAudit(t *testing.T) {
	f := seedAICRUDTeam(t, true)
	base := "/v1/teams/" + f.teamID + "/campaign-formats"

	assertErr(t, doRequest(t, f.h, http.MethodPost, base, f.bearer, map[string]any{"structure": map[string]any{"a": 1}}), http.StatusBadRequest, "invalid_campaign_format")
	assertErr(t, doRequest(t, f.h, http.MethodPost, base, f.bearer, map[string]any{"name": "n"}), http.StatusBadRequest, "invalid_campaign_format")
	assertErr(t, doRequest(t, f.h, http.MethodPost, base, f.bearer, map[string]any{"name": strings.Repeat("n", 121), "structure": map[string]any{"a": 1}}), http.StatusBadRequest, "invalid_campaign_format")
	assertErr(t, doRequest(t, f.h, http.MethodPost, base, f.bearer, map[string]any{"name": "n", "weekday": 7, "structure": map[string]any{"a": 1}}), http.StatusBadRequest, "invalid_campaign_format")
	assertErr(t, doRequest(t, f.h, http.MethodPost, base, f.bearer, map[string]any{"name": "n", "structure": map[string]any{"a": 1}, "required_hashtags": make([]string, 31)}), http.StatusBadRequest, "invalid_campaign_format")

	rec := doRequest(t, f.h, http.MethodPost, base, f.bearer, map[string]any{"name": "Weekly", "structure": map[string]any{"a": 1}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var created domain.CampaignFormat
	_ = json.NewDecoder(rec.Body).Decode(&created)

	assertErr(t, doRequest(t, f.h, http.MethodPatch, base+"/"+created.ID, f.bearer, map[string]any{"name": "", "structure": map[string]any{"a": 1}}), http.StatusBadRequest, "invalid_campaign_format")
	assertErr(t, doRequest(t, f.h, http.MethodPatch, base+"/"+uuid.NewString(), f.bearer, map[string]any{"name": "x", "structure": map[string]any{"a": 1}}), http.StatusNotFound, "campaign_format_not_found")
	if rec := doRequest(t, f.h, http.MethodPatch, base+"/"+created.ID, f.bearer, map[string]any{"name": "Monthly", "structure": map[string]any{"a": 1}}); rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(t, f.h, http.MethodDelete, base+"/"+created.ID, f.bearer, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rec.Code)
	}
	actions := auditActions(t, f)
	for _, want := range []string{"campaign_format.create", "campaign_format.update", "campaign_format.delete"} {
		if !hasAction(actions, want) {
			t.Fatalf("missing audit %q in %v", want, actions)
		}
	}
}

func TestStyleExampleValidationAndAudit(t *testing.T) {
	f := seedAICRUDTeam(t, true)
	base := "/v1/teams/" + f.teamID + "/style-examples"

	assertErr(t, doRequest(t, f.h, http.MethodPost, base, f.bearer, map[string]any{"content": "  "}), http.StatusBadRequest, "invalid_style_example")
	assertErr(t, doRequest(t, f.h, http.MethodPost, base, f.bearer, map[string]any{"content": strings.Repeat("x", 5001)}), http.StatusBadRequest, "invalid_style_example")
	assertErr(t, doRequest(t, f.h, http.MethodPost, base, f.bearer, map[string]any{"content": "ok", "notes": strings.Repeat("x", 1001)}), http.StatusBadRequest, "invalid_style_example")

	rec := doRequest(t, f.h, http.MethodPost, base, f.bearer, map[string]any{"platform": "mastodon", "content": "hello"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var created domain.StyleExample
	_ = json.NewDecoder(rec.Body).Decode(&created)
	if rec := doRequest(t, f.h, http.MethodDelete, base+"/"+created.ID, f.bearer, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rec.Code)
	}
	actions := auditActions(t, f)
	if !hasAction(actions, "style_example.create") || !hasAction(actions, "style_example.delete") {
		t.Fatalf("missing audit in %v", actions)
	}
}

func TestKnowledgeSourceValidationLimitsAndAudit(t *testing.T) {
	f := seedAICRUDTeam(t, true)
	base := "/v1/teams/" + f.teamID + "/knowledge-sources"

	assertErr(t, doRequest(t, f.h, http.MethodPost, base, f.bearer, map[string]any{"type": "text", "name": "n"}), http.StatusBadRequest, "invalid_knowledge_source")
	assertErr(t, doRequest(t, f.h, http.MethodPost, base, f.bearer, map[string]any{"type": "bogus", "name": "n", "content": "c"}), http.StatusBadRequest, "invalid_knowledge_source")
	assertErr(t, doRequest(t, f.h, http.MethodPost, base, f.bearer, map[string]any{"type": "text", "name": strings.Repeat("n", 201), "content": "c"}), http.StatusBadRequest, "invalid_knowledge_source")
	assertErr(t, doRequest(t, f.h, http.MethodPost, base, f.bearer, map[string]any{"type": "text", "name": "n", "content": strings.Repeat("x", 100_001)}), http.StatusBadRequest, "knowledge_source_too_large")

	var firstID string
	for i := 0; i < 50; i++ {
		rec := doRequest(t, f.h, http.MethodPost, base, f.bearer, map[string]any{"type": "text", "name": "n", "content": "c"})
		if rec.Code != http.StatusCreated {
			t.Fatalf("create #%d: %d %s", i, rec.Code, rec.Body.String())
		}
		if i == 0 {
			var ks domain.KnowledgeSource
			_ = json.NewDecoder(rec.Body).Decode(&ks)
			firstID = ks.ID
		}
	}
	assertErr(t, doRequest(t, f.h, http.MethodPost, base, f.bearer, map[string]any{"type": "text", "name": "n", "content": "c"}), http.StatusConflict, "knowledge_source_limit_reached")

	assertErr(t, doRequest(t, f.h, http.MethodPatch, base+"/"+uuid.NewString(), f.bearer, map[string]any{"type": "text", "name": "n", "content": "c"}), http.StatusNotFound, "knowledge_source_not_found")
	if rec := doRequest(t, f.h, http.MethodPatch, base+"/"+firstID, f.bearer, map[string]any{"type": "text", "name": "renamed", "content": "c"}); rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(t, f.h, http.MethodDelete, base+"/"+firstID, f.bearer, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rec.Code)
	}
	actions := auditActions(t, f)
	for _, want := range []string{"knowledge_source.create", "knowledge_source.update", "knowledge_source.delete"} {
		if !hasAction(actions, want) {
			t.Fatalf("missing audit %q", want)
		}
	}
}

func TestProactiveSettingsValidationDefaultsAndAudit(t *testing.T) {
	f := seedAICRUDTeam(t, true)
	path := "/v1/teams/" + f.teamID + "/proactive-settings"

	rec := doRequest(t, f.h, http.MethodGet, path, f.bearer, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET defaults: %d %s", rec.Code, rec.Body.String())
	}
	var def domain.ProactiveTriggerSettings
	_ = json.NewDecoder(rec.Body).Decode(&def)
	if def.ContentGapThresholdDays != 3 || def.MaxTriggersPerDay != 5 {
		t.Fatalf("defaults: %+v", def)
	}

	valid := map[string]any{"content_gap_threshold_days": 7, "max_triggers_per_day": 4, "cron_schedule": "0 * * * *"}
	bad := []map[string]any{
		{"content_gap_threshold_days": 0, "max_triggers_per_day": 4, "cron_schedule": "0 * * * *"},
		{"content_gap_threshold_days": 366, "max_triggers_per_day": 4, "cron_schedule": "0 * * * *"},
		{"content_gap_threshold_days": 7, "max_triggers_per_day": 0, "cron_schedule": "0 * * * *"},
		{"content_gap_threshold_days": 7, "max_triggers_per_day": 101, "cron_schedule": "0 * * * *"},
		{"content_gap_threshold_days": 7, "max_triggers_per_day": 4, "cron_schedule": "nonsense"},
	}
	for i, b := range bad {
		rec := doRequest(t, f.h, http.MethodPut, path, f.bearer, b)
		if rec.Code != http.StatusBadRequest || rec.Header().Get("X-Error-Code") != "invalid_proactive_settings" {
			t.Fatalf("bad[%d]: %d %q", i, rec.Code, rec.Header().Get("X-Error-Code"))
		}
	}
	if rec := doRequest(t, f.h, http.MethodPut, path, f.bearer, valid); rec.Code != http.StatusOK {
		t.Fatalf("valid PUT: %d %s", rec.Code, rec.Body.String())
	}
	if !hasAction(auditActions(t, f), "proactive_settings.update") {
		t.Fatal("missing proactive_settings.update audit")
	}
}

type failingProactiveStore struct{ store.Store }

func (failingProactiveStore) GetProactiveTriggerSettings(context.Context, string) (domain.ProactiveTriggerSettings, error) {
	return domain.ProactiveTriggerSettings{}, errors.New("not found in some weird db message")
}

func TestGetProactiveSettings_ErrorMessageTextIsNotNotFound(t *testing.T) {
	f := seedAICRUDTeam(t, true)
	wrapped := failingProactiveStore{Store: f.store}
	authSvc, err := auth.New(context.Background(), config.Config{}, wrapped)
	if err != nil {
		t.Fatal(err)
	}
	catalog, _ := i18n.Load()
	h := api.New(nil, wrapped, authSvc, provider.NewRegistry(provider.NewBlueskyProvider()), config.Config{}, nil, catalog, nil, nil).
		Handler(security.NewLimiter(10_000, 10_000), nil)
	rec := doRequest(t, h, http.MethodGet, "/v1/teams/"+f.teamID+"/proactive-settings", f.bearer, nil)
	assertErr(t, rec, http.StatusInternalServerError, "internal_error")
}
