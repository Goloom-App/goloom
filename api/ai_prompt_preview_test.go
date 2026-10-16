package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.f4mily.net/goloom/internal/ai"
	"git.f4mily.net/goloom/internal/aicontext"
	"git.f4mily.net/goloom/internal/domain"
)

func TestAIPromptPreviewMatchesExecution(t *testing.T) {
	for _, refine := range []bool{false, true} {
		name := "generation"
		if refine {
			name = "refine"
		}
		t.Run(name, func(t *testing.T) {
			fx := seedAICRUDTeam(t, true)
			ctx := context.Background()
			var selectedID string
			for _, item := range []struct {
				provider string
				limit    int
			}{{"mastodon", 500}, {"mastodon", 5000}, {"bluesky", 300}} {
				limit := item.limit
				account, err := fx.store.CreateAccount(ctx, fx.teamID, domain.ConnectedAccount{
					Provider: item.provider, Username: item.provider, AuthType: domain.AccountAuthTypeOAuthToken,
					InstanceURL: "https://example.org", AccessToken: "test",
				})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := fx.store.UpdateAccount(ctx, fx.teamID, account.ID, domain.UpdateAccountInput{MaxCharsOverride: &limit}); err != nil {
					t.Fatal(err)
				}
				if limit == 5000 {
					selectedID = account.ID
				}
			}
			p := map[string]any{"target_account_ids": []string{selectedID}, "schedule": false, "occasion": "Meetup editor@example.org", "prompt_hint": "Contact editor@example.org"}
			if refine {
				p["refine_content"] = true
				p["source_content"] = "Existing draft"
			}
			rec := doRequest(t, fx.h, http.MethodPost, "/v1/teams/"+fx.teamID+"/ai/prompt-preview", fx.bearer, map[string]any{"params": p})
			if rec.Code != http.StatusOK {
				t.Fatalf("preview status %d: %s", rec.Code, rec.Body.String())
			}
			var preview struct {
				System string `json:"system_prompt"`
				Task   string `json:"generation_prompt"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &preview); err != nil {
				t.Fatal(err)
			}
			var requests []struct {
				Messages []struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"messages"`
			}
			llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Messages []struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					} `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				requests = append(requests, request)
				content, _ := json.Marshal(map[string]any{"title": "Meetup", "content": strings.Repeat("x", 4500), "hashtags": []string{}})
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": string(content)}, "finish_reason": "stop"}}})
			}))
			defer llm.Close()
			aiContext, err := aicontext.Build(ctx, fx.store, fx.teamID, aicontext.Full())
			if err != nil {
				t.Fatal(err)
			}
			payload, _ := json.Marshal(map[string]any{"params": p})
			_, err = ai.NewService("127.0.0.1").RunJob(ctx, domain.AIJob{Type: domain.AIJobTypeVoiceEngine, Payload: payload}, domain.AIServiceConfig{Provider: ai.ProviderOpenAI, Model: "test", BaseURL: llm.URL, APIKey: "test"}, aiContext)
			if err != nil {
				t.Fatal(err)
			}
			if len(requests) == 0 {
				t.Fatal("no model request")
			}
			for _, message := range requests[0].Messages {
				if message.Role == "system" && message.Content != preview.System {
					t.Error("preview system differs from execution")
				}
				if message.Role == "user" && message.Content != preview.Task {
					t.Error("preview task differs from execution")
				}
			}
			want := "Multi-account output rules:"
			if refine {
				want = "Refine an existing draft"
			}
			for _, text := range []string{want, "id=" + selectedID, "[redacted]"} {
				if !strings.Contains(preview.Task, text) {
					t.Errorf("preview missing %q", text)
				}
			}
			if strings.Contains(preview.Task, "editor@example.org") {
				t.Error("preview exposes redacted PII")
			}
		})
	}
}

func TestAIPromptPreviewRejectsInvalidParams(t *testing.T) {
	fx := seedAICRUDTeam(t, true)
	for _, p := range []any{map[string]any{}, map[string]any{"target_account_ids": []string{"foreign-account"}}, []string{"invalid"}} {
		rec := doRequest(t, fx.h, http.MethodPost, "/v1/teams/"+fx.teamID+"/ai/prompt-preview", fx.bearer, map[string]any{"params": p})
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid AI prompt parameters") {
			t.Errorf("invalid params: status=%d body=%s", rec.Code, rec.Body.String())
		}
	}
}
