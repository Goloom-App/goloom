package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
)

const groundedRSSPost = "Release 2 ships Friday with offline mode. Read it: https://example.org/release"

func rssVoiceResponse(t *testing.T, content string, overrides map[string]string) Response {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"title": "Release 2", "content": content, "account_content_override": overrides, "hashtags": []string{}})
	if err != nil {
		t.Fatal(err)
	}
	return Response{Content: string(raw)}
}

func TestVoiceEngineShortSourceGroundedPosts(t *testing.T) {
	if len([]rune(groundedRSSPost)) != 78 {
		t.Fatal("fixture must have 78 characters")
	}
	for _, account := range []domain.AIAccountSummary{
		{ID: "primary", Provider: "mastodon", MaxChars: 500},
		{ID: "primary", Provider: "bluesky", MaxChars: 300},
		{ID: "primary", Provider: "friendica", MaxChars: 5000},
	} {
		for _, mode := range []string{"rss_source", "rss_metadata", "recurring_main", "recurring_announcement", "generic"} {
			t.Run(fmt.Sprintf("%s/%s", account.Provider, mode), func(t *testing.T) {
				aiContext := testContext()
				aiContext.Accounts = []domain.AIAccountSummary{account}
				p := params{"schedule": false, "target_account_ids": []string{"primary"}}
				switch mode {
				case "rss_source":
					p["rss_article_title"] = "Release 2"
					p["rss_article_link"] = "https://example.org/release"
					p["rss_article_content"] = "Release 2 ships Friday with offline mode."
				case "rss_metadata":
					p["rss_automation"] = map[string]any{"feed_id": "feed-1"}
				case "recurring_main", "recurring_announcement":
					p["recurring_post_kind"] = strings.TrimPrefix(mode, "recurring_")
					p["source_content"] = groundedRSSPost
				case "generic":
					// A caller-provided flag must not bypass generic minimum validation.
					p["rss_generation"] = true
				}
				client := &scriptedClient{responses: []Response{rssVoiceResponse(t, groundedRSSPost, nil)}}
				raw, err := runVoiceEngine(context.Background(), client, domain.AIJob{Type: domain.AIJobTypeVoiceEngine}, aiContext, p)
				if mode == "generic" {
					if err == nil || !strings.Contains(err.Error(), "Primary content is too short (78 chars)") {
						t.Fatalf("generic error = %v", err)
					}
					if len(client.requests) != voiceEngineMaxRetries+1 {
						t.Fatalf("generic calls = %d", len(client.requests))
					}
					return
				}
				if err != nil {
					t.Fatalf("grounded post rejected after %d calls: %v", len(client.requests), err)
				}
				if len(client.requests) != 1 {
					t.Fatalf("calls = %d, want 1", len(client.requests))
				}
				var result voiceEngineResult
				if err := json.Unmarshal(raw, &result); err != nil {
					t.Fatal(err)
				}
				if result.Content != groundedRSSPost {
					t.Fatalf("content changed: %q", result.Content)
				}
			})
		}
	}
}

func TestVoiceEngineRSSRetryStaysSourceGrounded(t *testing.T) {
	client := &scriptedClient{responses: []Response{
		rssVoiceResponse(t, "", nil),
		rssVoiceResponse(t, groundedRSSPost, nil),
	}}
	_, err := runVoiceEngine(context.Background(), client, domain.AIJob{Type: domain.AIJobTypeVoiceEngine}, testContext(), params{"rss_article_title": "Release 2", "schedule": false, "target_account_ids": []string{"acc-1", "acc-2"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 2 {
		t.Fatalf("calls = %d, want 2", len(client.requests))
	}
	for i, req := range client.requests {
		prompt := req.Messages[len(req.Messages)-1].Content
		for _, unwanted := range []string{"targeting roughly", "must be long", "aim for at least"} {
			if strings.Contains(prompt, unwanted) {
				t.Errorf("request %d demands filler: %q", i, unwanted)
			}
		}
		if !strings.Contains(prompt, "as long as the source supports, max 500") {
			t.Errorf("request %d missing source-based length guidance", i)
		}
	}
}

func TestVoiceEngineRSSKeepsLimitsAndOverrides(t *testing.T) {
	client := &scriptedClient{responses: []Response{
		rssVoiceResponse(t, strings.Repeat("Long source text. ", 40), nil),
		rssVoiceResponse(t, strings.Repeat("Long source text. ", 40), map[string]string{"acc-2": groundedRSSPost}),
	}}
	raw, err := runVoiceEngine(context.Background(), client, domain.AIJob{Type: domain.AIJobTypeVoiceEngine}, testContext(), params{"rss_article_title": "Release 2", "schedule": false, "target_account_ids": []string{"acc-1", "acc-2"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 2 {
		t.Fatalf("calls = %d, want retry for missing lower-limit override", len(client.requests))
	}
	var result voiceEngineResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if len([]rune(result.Content)) > 500 || result.AccountContentOverride["acc-2"] != groundedRSSPost {
		t.Fatalf("limits/override not preserved: %+v", result)
	}
}
