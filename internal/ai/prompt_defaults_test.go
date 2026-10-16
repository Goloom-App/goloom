package ai

import (
	"strings"
	"testing"
	"unicode/utf8"

	"git.f4mily.net/goloom/internal/domain"
)

func TestFormatKnowledgeSourceTruncatesRuneSafe(t *testing.T) {
	// 3 bytes per rune would split mid-rune at a byte index of 4000.
	content := strings.Repeat("Größe😀", 1000)
	out := formatKnowledgeSource(domain.KnowledgeSource{Name: "n", Content: content})
	if !utf8.ValidString(out) {
		t.Fatal("knowledge source is not valid UTF-8")
	}
	if !strings.Contains(out, "…") {
		t.Fatal("expected ellipsis after truncation")
	}
	short := formatKnowledgeSource(domain.KnowledgeSource{Name: "n", Content: "Grüße"})
	if strings.Contains(short, "…") {
		t.Fatalf("short content must not be truncated: %s", short)
	}
}

func TestPlatformCharLimit(t *testing.T) {
	cases := []struct {
		name     string
		accounts []domain.AIAccountSummary
		platform string
		want     int
	}{
		{"no accounts falls back to provider default", nil, "bluesky", 300},
		{"unknown platform falls back to 500", nil, "pixelfed", 500},
		{"account value wins", []domain.AIAccountSummary{{Provider: "mastodon", MaxChars: 1500}}, "mastodon", 1500},
		{"smallest of several accounts", []domain.AIAccountSummary{
			{Provider: "mastodon", MaxChars: 1500}, {Provider: "Mastodon", MaxChars: 800},
		}, "mastodon", 800},
		{"other providers ignored", []domain.AIAccountSummary{{Provider: "bluesky", MaxChars: 100}}, "mastodon", 500},
		{"zero MaxChars ignored", []domain.AIAccountSummary{{Provider: "mastodon"}}, "mastodon", 500},
		{"empty platform", nil, "", 500},
	}
	for _, tc := range cases {
		got := platformCharLimit(domain.AIContext{Accounts: tc.accounts}, tc.platform)
		if got != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestGenerationPromptHonorsAccountOverride(t *testing.T) {
	ctx := domain.AIContext{Accounts: []domain.AIAccountSummary{{Provider: "mastodon", MaxChars: 1500}}}
	prompt := buildGenerationPrompt(ctx, params{}, "mastodon")
	if !strings.Contains(prompt, "Character limit: 1500") {
		t.Fatalf("expected override limit in prompt:\n%s", prompt)
	}
}

func TestHashtagRuleConsistentAcrossPrompts(t *testing.T) {
	cases := []struct {
		name        string
		platform    string
		maxHashtags int
		taskWant    string
		sysWant     string
		sysNot      string
	}{
		{"configured", "mastodon", 2, "exactly 2", "up to 2", "up to 0"},
		{"configured many", "mastodon", 5, "3 to 5", "up to 5", "up to 0"},
		{"mastodon default", "mastodon", 0, "1-3 readable CamelCase hashtags at the end", "platform defaults", "up to 0"},
		{"bluesky default", "bluesky", 0, "at most 2", "platform defaults", "up to 0"},
		{"friendica default", "friendica", 0, "only relevant", "platform defaults", "up to 0"},
		{"unknown platform default", "pixelfed", 0, "at most 2", "platform defaults", "up to 0"},
	}
	for _, tc := range cases {
		ctx := domain.AIContext{Profile: &domain.TeamProfile{StyleMetadata: domain.StyleMetadata{MaxHashtags: tc.maxHashtags}}}
		task := buildGenerationPrompt(ctx, params{}, tc.platform)
		sys := BuildSystemPrompt(ctx)
		if !strings.Contains(task, tc.taskWant) {
			t.Errorf("%s: task prompt missing %q:\n%s", tc.name, tc.taskWant, task)
		}
		if !strings.Contains(sys, tc.sysWant) {
			t.Errorf("%s: system prompt missing %q:\n%s", tc.name, tc.sysWant, sys)
		}
		if strings.Contains(sys, tc.sysNot) {
			t.Errorf("%s: system prompt contains %q", tc.name, tc.sysNot)
		}
	}
}

func TestSystemPromptDefaultsOnlyWhereProfileEmpty(t *testing.T) {
	const (
		tone = "natural, human tone"
		hook = "Open with a hook"
		cta  = "at most one clear call to action"
	)
	cases := []struct {
		name     string
		ctx      domain.AIContext
		want     []string
		wantNot  []string
		wantLang string
	}{
		{
			name:     "no profile",
			ctx:      domain.AIContext{Team: domain.Team{Name: "T"}},
			want:     []string{tone, hook, cta},
			wantLang: "match the language of the request",
		},
		{
			name: "partial: only persona",
			ctx: domain.AIContext{Profile: &domain.TeamProfile{StyleMetadata: domain.StyleMetadata{
				Identity: &domain.BrandIdentity{Persona: "A baker."}, PreferredLanguage: "de",
			}}},
			want:     []string{hook, cta},
			wantNot:  []string{tone},
			wantLang: "Language: de",
		},
		{
			name: "hook and CTA set",
			ctx: domain.AIContext{Profile: &domain.TeamProfile{StyleMetadata: domain.StyleMetadata{
				ReachStrategy: &domain.BrandReachStrategy{HookStyle: "question", CTAFocus: "visit shop"},
			}}},
			want:     []string{tone},
			wantNot:  []string{hook, cta},
			wantLang: "match the language of the request",
		},
		{
			name: "tonality set",
			ctx: domain.AIContext{Profile: &domain.TeamProfile{StyleMetadata: domain.StyleMetadata{
				Tonality: "formal",
			}}},
			want:    []string{"formal"},
			wantNot: []string{tone},
		},
	}
	for _, tc := range cases {
		sys := BuildSystemPrompt(tc.ctx)
		for _, w := range tc.want {
			if !strings.Contains(sys, w) {
				t.Errorf("%s: missing %q:\n%s", tc.name, w, sys)
			}
		}
		for _, w := range tc.wantNot {
			if strings.Contains(sys, w) {
				t.Errorf("%s: unexpected %q:\n%s", tc.name, w, sys)
			}
		}
		if tc.wantLang != "" && !strings.Contains(sys, tc.wantLang) {
			t.Errorf("%s: missing language %q:\n%s", tc.name, tc.wantLang, sys)
		}
		if strings.Contains(sys, "unspecified") {
			t.Errorf("%s: leaked 'unspecified'", tc.name)
		}
	}
}
