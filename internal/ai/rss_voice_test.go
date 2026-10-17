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

func TestVoiceEngineRSSMetadataStaysOutOfPrompt(t *testing.T) {
	for _, skeleton := range []string{
		"Release 2 https://example.org/release",
		strings.Repeat("Source summary. ", 1000) + "rss_automation feed_id fallback_content",
	} {
		t.Run(fmt.Sprintf("skeleton_%d_chars", len(skeleton)), func(t *testing.T) {
			meta := map[string]any{
				"feed_id": "private-feed-id", "item_key": "private-item-key",
				"output_mode": "draft", "scheduled_at": "2026-10-17T12:00:00Z",
				"fallback_content": skeleton,
			}
			payload, err := json.Marshal(map[string]any{"params": map[string]any{
				"target_account_ids": []string{"acc-1"}, "schedule": false,
				"rss_article_title": "Release 2", "rss_article_link": "https://example.org/release",
				"rss_article_content": "Release 2 ships Friday with offline mode.",
				"post_skeleton":       skeleton, "rss_automation": meta,
				"recurring_automation": map[string]any{"template_id": "private-template-id"},
			}})
			if err != nil {
				t.Fatal(err)
			}
			job := domain.AIJob{Type: domain.AIJobTypeVoiceEngine, Payload: payload}
			p, err := parseJobParams(job.Payload)
			if err != nil {
				t.Fatal(err)
			}
			before, _ := json.Marshal(p)
			client := &scriptedClient{responses: []Response{rssVoiceResponse(t, strings.Repeat("Release 2 ships Friday with offline mode. ", 11), nil)}}
			if _, err := runVoiceEngine(context.Background(), client, job, testContext(), p); err != nil {
				t.Fatal(err)
			}
			if len(client.requests) != 1 {
				t.Fatalf("calls = %d, want 1", len(client.requests))
			}
			for _, message := range client.requests[0].Messages {
				prompt := message.Content
				if message.Role == RoleUser {
					wrappedSkeleton := wrapUntrusted(skeleton)
					if strings.Count(prompt, wrappedSkeleton) != 1 {
						t.Error("skeleton must appear once in its untrusted wrapper")
					}
					prompt = strings.ReplaceAll(prompt, wrappedSkeleton, "")
				}
				for _, unwanted := range []string{"rss_automation", "feed_id", "fallback_content", "private-feed-id", "private-item-key", "recurring_automation", "private-template-id"} {
					if strings.Contains(prompt, unwanted) {
						t.Errorf("metadata leaked outside untrusted skeleton: %q", unwanted)
					}
				}
			}
			after, _ := json.Marshal(p)
			if string(before) != string(after) {
				t.Fatal("prompt rendering changed callback params")
			}
			callbackParams, err := parseJobParams(job.Payload)
			if err != nil {
				t.Fatal(err)
			}
			callbackMeta := callbackParams.nested("rss_automation")
			if callbackMeta.str("feed_id") != "private-feed-id" || callbackMeta.str("fallback_content") != skeleton {
				t.Fatal("callback metadata lost")
			}
		})
	}
}
