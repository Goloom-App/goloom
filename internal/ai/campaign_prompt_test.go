package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
)

func TestCampaignPromptRetainsPlatformMinimumWithSelectedAccount(t *testing.T) {
	aiContext := testContext()
	aiContext.Accounts = []domain.AIAccountSummary{
		{ID: "short", Provider: "mastodon", MaxChars: 500},
		{ID: "long", Provider: "mastodon", MaxChars: 5000},
	}
	aiContext.CampaignFormats = []domain.CampaignFormat{{ID: "weekly", Name: "Weekly", IsActive: true, RequiredHashtags: []string{"#weekly"}}}
	// Force a length retry so both the initial prompt and retry must agree
	// with the campaign validator, even when only the long account is selected.
	// The validator appends the missing required hashtag before checking length.
	tooLong := strings.Repeat("x", 493)
	valid := strings.Repeat("x", 492)
	client := &scriptedClient{responses: []Response{
		{Content: `{"content":"` + tooLong + `","hashtags":[]}`},
		{Content: `{"content":"` + valid + `","hashtags":[]}`},
	}}
	p := params{"campaign_format_id": "weekly", "platform": "mastodon", "target_account_ids": []string{"long"}}
	raw, err := runCampaignAutopilot(context.Background(), client, domain.AIJob{Type: domain.AIJobTypeCampaignAutopilot}, aiContext, p)
	if err != nil {
		t.Fatal(err)
	}
	var result campaignResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result.Content != valid+" #weekly" || len(client.requests) != 2 {
		t.Fatalf("expected retry at 501 characters and acceptance at 500; result=%q requests=%d", result.Content, len(client.requests))
	}
	for i, request := range client.requests {
		prompt := request.Messages[len(request.Messages)-1].Content
		if !strings.Contains(prompt, "Character limit: 500\n") || strings.Contains(prompt, "Character limit: 5000") {
			t.Errorf("request %d: campaign prompt disagrees with validator's 500-character limit", i)
		}
	}
}
