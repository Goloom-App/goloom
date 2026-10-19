package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
)

func TestKnowledgeSelectionTeamBoundaryAndPatch(t *testing.T) {
	fx := seedAICRUDTeam(t, true)
	ctx := context.Background()
	source, err := fx.store.CreateKnowledgeSource(ctx, fx.teamID, domain.KnowledgeSource{Name: "Brand", Type: "text", Content: "Brand facts"})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"campaign-formats", "rss-feeds"} {
		for _, ids := range [][]string{nil, {}, {source.ID}, {"foreign-or-deleted"}} {
			input := map[string]any{"name": "Selected", "structure": map[string]any{"topic": "Topic"}, "feed_url": "https://example.org/feed", "knowledge_source_ids": ids}
			rec := doRequest(t, fx.h, http.MethodPost, "/v1/teams/"+fx.teamID+"/"+path, fx.bearer, input)
			want := http.StatusCreated
			if len(ids) > 0 && ids[0] != "" && ids[0] != source.ID {
				want = http.StatusBadRequest
			}
			if rec.Code != want {
				t.Fatalf("%s ids=%v: %d %s", path, ids, rec.Code, rec.Body.String())
			}
		}
	}
	format, err := fx.store.CreateCampaignFormat(ctx, fx.teamID, domain.CampaignFormat{Name: "Patch format", Structure: json.RawMessage(`{}`), KnowledgeSourceIDs: []string{source.ID}})
	if err != nil {
		t.Fatal(err)
	}
	for _, selection := range []struct {
		set bool
		ids []string
	}{{false, nil}, {true, []string{}}, {true, nil}} {
		patch := map[string]any{"name": "Patch format", "structure": map[string]any{"topic": "Topic"}}
		if selection.set {
			patch["knowledge_source_ids"] = selection.ids
		}
		rec := doRequest(t, fx.h, http.MethodPatch, "/v1/teams/"+fx.teamID+"/campaign-formats/"+format.ID, fx.bearer, patch)
		if rec.Code != http.StatusOK {
			t.Fatalf("campaign patch: %d %s", rec.Code, rec.Body.String())
		}
		var got domain.CampaignFormat
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if !selection.set && len(got.KnowledgeSourceIDs) != 1 {
			t.Fatal("omitted campaign selection changed")
		}
		if selection.set && selection.ids == nil && got.KnowledgeSourceIDs != nil {
			t.Fatal("null campaign selection did not inherit")
		}
		if selection.set && selection.ids != nil && (got.KnowledgeSourceIDs == nil || len(got.KnowledgeSourceIDs) != 0) {
			t.Fatal("empty campaign selection did not disable")
		}
	}
	rec := doRequest(t, fx.h, http.MethodPost, "/v1/teams/"+fx.teamID+"/rss-feeds", fx.bearer, map[string]any{"name": "Patch", "feed_url": "https://example.org/rss", "knowledge_source_ids": []string{source.ID}})
	var feed domain.RSSFeedConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &feed); err != nil {
		t.Fatal(err)
	}
	for _, patch := range []map[string]any{{"name": "Unchanged selection"}, {"knowledge_source_ids": []string{}}, {"knowledge_source_ids": nil}} {
		rec = doRequest(t, fx.h, http.MethodPatch, "/v1/teams/"+fx.teamID+"/rss-feeds/"+feed.ID, fx.bearer, patch)
		if rec.Code != http.StatusOK {
			t.Fatalf("patch: %d %s", rec.Code, rec.Body.String())
		}
		var got domain.RSSFeedConfig
		_ = json.Unmarshal(rec.Body.Bytes(), &got)
		if _, set := patch["knowledge_source_ids"]; !set && len(got.KnowledgeSourceIDs) != 1 {
			t.Fatal("omitted selection changed")
		}
		if ids, set := patch["knowledge_source_ids"]; set && ids == nil && got.KnowledgeSourceIDs != nil {
			t.Fatal("null did not restore inheritance")
		}
		if ids, set := patch["knowledge_source_ids"]; set && ids != nil && (got.KnowledgeSourceIDs == nil || len(got.KnowledgeSourceIDs) != 0) {
			t.Fatal("empty did not disable knowledge")
		}
	}
}

func TestSelectedKnowledgePromptAndDeletedPolicy(t *testing.T) {
	fx := seedAICRUDTeam(t, true)
	ctx := context.Background()
	selected, err := fx.store.CreateKnowledgeSource(ctx, fx.teamID, domain.KnowledgeSource{Name: "Selected", Type: "text", Content: "SELECTED_BRAND_FACT"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = fx.store.CreateKnowledgeSource(ctx, fx.teamID, domain.KnowledgeSource{Name: "Other", Type: "text", Content: "UNRELATED_BRAND_FACT"})
	if err != nil {
		t.Fatal(err)
	}
	account, err := fx.store.CreateAccount(ctx, fx.teamID, domain.ConnectedAccount{Provider: "mastodon", Username: "test", AuthType: domain.AccountAuthTypeOAuthToken, AccessToken: "test", InstanceURL: "https://example.org"})
	if err != nil {
		t.Fatal(err)
	}
	format, err := fx.store.CreateCampaignFormat(ctx, fx.teamID, domain.CampaignFormat{Name: "Selected", IsActive: true, Structure: json.RawMessage(`{}`), KnowledgeSourceIDs: []string{selected.ID}})
	if err != nil {
		t.Fatal(err)
	}
	p := map[string]any{"target_account_ids": []string{account.ID}, "campaign_format_id": format.ID, "rss_article_title": "Source title", "rss_article_content": "Source facts only.", "rss_article_link": "https://example.org/item"}
	rec := doRequest(t, fx.h, http.MethodPost, "/v1/teams/"+fx.teamID+"/ai/prompt-preview", fx.bearer, map[string]any{"params": p})
	if rec.Code != http.StatusOK {
		t.Fatalf("preview: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "SELECTED_BRAND_FACT") || strings.Contains(rec.Body.String(), "UNRELATED_BRAND_FACT") {
		t.Fatal("wrong knowledge entered prompt")
	}
	if !strings.Contains(rec.Body.String(), "Source facts only") || !strings.Contains(rec.Body.String(), "every specific claim must come from here") {
		t.Fatal("RSS factual boundary missing")
	}
	if err := fx.store.DeleteKnowledgeSource(ctx, fx.teamID, selected.ID); err != nil {
		t.Fatal(err)
	}
	rec = doRequest(t, fx.h, http.MethodPost, "/v1/teams/"+fx.teamID+"/ai/prompt-preview", fx.bearer, map[string]any{"params": p})
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Selected knowledge sources are unavailable") {
		t.Fatalf("deleted policy: %d %s", rec.Code, rec.Body.String())
	}
}
