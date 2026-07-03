package ai

import (
	"context"
	"strings"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

// ---- zeroPad (campaign.go) ----

func TestZeroPad(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{1, "01"},
		{9, "09"},
		{10, "10"},
		{31, "31"},
		{0, "00"},
	}
	for _, tc := range cases {
		if got := zeroPad(tc.in); got != tc.want {
			t.Errorf("zeroPad(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// ---- clampDay (campaign.go) ----

func TestClampDay(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, 1},
		{-5, 1},
		{1, 1},
		{15, 15},
		{31, 31},
		{32, 31},
		{100, 31},
	}
	for _, tc := range cases {
		if got := clampDay(tc.in); got != tc.want {
			t.Errorf("clampDay(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// ---- clampMonth (campaign.go) ----

func TestClampMonth(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, 1},
		{-1, 1},
		{1, 1},
		{6, 6},
		{12, 12},
		{13, 12},
		{99, 12},
	}
	for _, tc := range cases {
		if got := clampMonth(tc.in); got != tc.want {
			t.Errorf("clampMonth(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// ---- renderTextTemplate (campaign.go) ----

func TestRenderTextTemplate_BasicPlaceholders(t *testing.T) {
	restore := nowFunc
	nowFunc = func() time.Time { return time.Date(2026, 3, 7, 0, 0, 0, 0, time.UTC) }
	defer func() { nowFunc = restore }()

	got := renderTextTemplate("{year}-{month}-{day}", params{}, nil)
	if got != "2026-03-07" {
		t.Fatalf("renderTextTemplate = %q, want 2026-03-07", got)
	}
}

func TestRenderTextTemplate_DayOffset(t *testing.T) {
	restore := nowFunc
	nowFunc = func() time.Time { return time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC) }
	defer func() { nowFunc = restore }()

	got := renderTextTemplate("day: {day+2}", params{}, nil)
	if got != "day: 12" {
		t.Fatalf("renderTextTemplate day offset = %q, want 'day: 12'", got)
	}
}

func TestRenderTextTemplate_MonthOffset(t *testing.T) {
	restore := nowFunc
	nowFunc = func() time.Time { return time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC) }
	defer func() { nowFunc = restore }()

	got := renderTextTemplate("month: {month-1}", params{}, nil)
	if got != "month: 05" {
		t.Fatalf("renderTextTemplate month offset = %q, want 'month: 05'", got)
	}
}

func TestRenderTextTemplate_UsesScheduledAtWhenProvided(t *testing.T) {
	// nowFunc should not matter when scheduledAt is given.
	restore := nowFunc
	nowFunc = func() time.Time { return time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC) }
	defer func() { nowFunc = restore }()

	at := time.Date(2026, 12, 25, 0, 0, 0, 0, time.UTC)
	got := renderTextTemplate("{year}-{month}-{day}", params{}, &at)
	if got != "2026-12-25" {
		t.Fatalf("renderTextTemplate with scheduledAt = %q, want 2026-12-25", got)
	}
}

func TestRenderTextTemplate_EmptyStringPassthrough(t *testing.T) {
	if got := renderTextTemplate("", params{}, nil); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestRenderTextTemplate_ClampsDay(t *testing.T) {
	restore := nowFunc
	nowFunc = func() time.Time { return time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC) }
	defer func() { nowFunc = restore }()

	// day+5 = 35, clamped to 31.
	got := renderTextTemplate("{day+5}", params{}, nil)
	if got != "31" {
		t.Fatalf("renderTextTemplate clamped day = %q, want 31", got)
	}
}

// ---- intval (params.go) ----

func TestIntval_Float64Value(t *testing.T) {
	p := params{"count": float64(7)}
	if got := p.intval("count", 1); got != 7 {
		t.Fatalf("intval float64 = %d, want 7", got)
	}
}

func TestIntval_IntValue(t *testing.T) {
	p := params{"count": 5}
	if got := p.intval("count", 1); got != 5 {
		t.Fatalf("intval int = %d, want 5", got)
	}
}

func TestIntval_StringValue(t *testing.T) {
	p := params{"count": "42"}
	if got := p.intval("count", 1); got != 42 {
		t.Fatalf("intval string = %d, want 42", got)
	}
}

func TestIntval_MissingKeyReturnsFallback(t *testing.T) {
	p := params{}
	if got := p.intval("count", 99); got != 99 {
		t.Fatalf("intval missing = %d, want 99 (fallback)", got)
	}
}

func TestIntval_InvalidStringReturnsFallback(t *testing.T) {
	p := params{"count": "not-a-number"}
	if got := p.intval("count", 3); got != 3 {
		t.Fatalf("intval invalid string = %d, want 3 (fallback)", got)
	}
}

// ---- parseClock (slots.go) ----

func TestParseClock_ValidTimes(t *testing.T) {
	cases := []struct {
		raw          string
		hour, minute int
	}{
		{"09:00", 9, 0},
		{"14:30", 14, 30},
		{"00:00", 0, 0},
		{"23:59", 23, 59},
	}
	for _, tc := range cases {
		ct, ok := parseClock(tc.raw)
		if !ok {
			t.Errorf("parseClock(%q) returned false", tc.raw)
			continue
		}
		if ct.hour != tc.hour || ct.minute != tc.minute {
			t.Errorf("parseClock(%q) = {%d:%d}, want {%d:%d}", tc.raw, ct.hour, ct.minute, tc.hour, tc.minute)
		}
	}
}

func TestParseClock_InvalidInputs(t *testing.T) {
	invalid := []string{
		"",
		"9",
		"24:00",
		"00:60",
		"abc:def",
		"-1:00",
	}
	for _, raw := range invalid {
		if _, ok := parseClock(raw); ok {
			t.Errorf("parseClock(%q) should return false", raw)
		}
	}
}

// ---- formatKnowledgeSource (prompts.go) ----

func TestFormatKnowledgeSource_WithURLAndContent(t *testing.T) {
	ks := domain.KnowledgeSource{
		Name:      "Company FAQ",
		SourceURL: "https://example.com/faq",
		Content:   "We are a maker collective.",
	}
	out := formatKnowledgeSource(ks)
	if !strings.Contains(out, "Company FAQ") {
		t.Fatal("missing name")
	}
	if !strings.Contains(out, "https://example.com/faq") {
		t.Fatal("missing URL")
	}
	if !strings.Contains(out, "We are a maker collective.") {
		t.Fatal("missing content")
	}
}

func TestFormatKnowledgeSource_EmptyContent(t *testing.T) {
	ks := domain.KnowledgeSource{Name: "FAQ", Content: ""}
	out := formatKnowledgeSource(ks)
	if !strings.Contains(out, "No extracted content") {
		t.Fatalf("expected 'No extracted content' for empty source, got:\n%s", out)
	}
}

func TestFormatKnowledgeSource_LongContentTruncated(t *testing.T) {
	ks := domain.KnowledgeSource{Name: "Big", Content: strings.Repeat("x", 5000)}
	out := formatKnowledgeSource(ks)
	// Content should be truncated at 4000 chars with ellipsis.
	if !strings.Contains(out, "…") {
		t.Fatal("expected ellipsis in truncated content")
	}
}

func TestFormatKnowledgeSource_EmptyNameFallsBackToSource(t *testing.T) {
	ks := domain.KnowledgeSource{Name: "", Content: "Some text."}
	out := formatKnowledgeSource(ks)
	if !strings.Contains(out, "[source]") {
		t.Fatalf("expected [source] fallback for empty name, got:\n%s", out)
	}
}

// ---- webPageSourceSection (prompts.go) ----

func TestWebPageSourceSection_AllFields(t *testing.T) {
	p := params{
		"source_url":     "https://example.com/post",
		"page_title":     "Test Article",
		"source_content": "This is the article text.",
	}
	out := webPageSourceSection(p)
	for _, want := range []string{"https://example.com/post", "Test Article", "This is the article text."} {
		if !strings.Contains(out, want) {
			t.Fatalf("webPageSourceSection missing %q", want)
		}
	}
}

func TestWebPageSourceSection_URLOnlyNoContent(t *testing.T) {
	p := params{"source_url": "https://example.com/post"}
	out := webPageSourceSection(p)
	if !strings.Contains(out, "could not be extracted") {
		t.Fatalf("expected 'could not be extracted' message for URL without content, got:\n%s", out)
	}
}

// ---- buildVibePreviewPrompt (prompts.go) ----

func TestBuildVibePreviewPrompt_ContainsTeamName(t *testing.T) {
	ctx := testContext()
	prompt := buildVibePreviewPrompt(ctx)
	if !strings.Contains(prompt, "Testteam") {
		t.Fatalf("vibe preview prompt missing team name:\n%s", prompt)
	}
}

func TestBuildVibePreviewPrompt_ContainsJSONInstruction(t *testing.T) {
	prompt := buildVibePreviewPrompt(testContext())
	if !strings.Contains(prompt, "summary") {
		t.Fatalf("vibe preview prompt missing summary instruction:\n%s", prompt)
	}
}

func TestBuildVibePreviewPrompt_EmptyTeamNameFallback(t *testing.T) {
	ctx := testContext()
	ctx.Team.Name = ""
	prompt := buildVibePreviewPrompt(ctx)
	if !strings.Contains(prompt, "unknown team") {
		t.Fatalf("expected 'unknown team' fallback, got:\n%s", prompt)
	}
}

// ---- buildProfileAssistantPrompt (prompts.go) ----

func TestBuildProfileAssistantPrompt_ValidBrief(t *testing.T) {
	p := params{"brief": "A dentist practice in Berlin, friendly and informative."}
	prompt, err := buildProfileAssistantPrompt(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "A dentist practice in Berlin") {
		t.Fatalf("prompt missing brief text:\n%s", prompt)
	}
}

func TestBuildProfileAssistantPrompt_EmptyBriefReturnsError(t *testing.T) {
	_, err := buildProfileAssistantPrompt(params{})
	if err == nil {
		t.Fatal("expected error for empty brief")
	}
}

func TestBuildProfileAssistantPrompt_WithExamples(t *testing.T) {
	p := params{
		"brief":    "A solo indie dev.",
		"examples": []any{"Shipped it! 🚀", "v2.0 is out."},
	}
	prompt, err := buildProfileAssistantPrompt(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "Shipped it") {
		t.Fatalf("prompt missing example text:\n%s", prompt)
	}
}

func TestBuildProfileAssistantPrompt_LanguageParam(t *testing.T) {
	p := params{"brief": "A tech blog.", "language": "en"}
	prompt, err := buildProfileAssistantPrompt(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, `"preferred_language": "en"`) {
		t.Fatalf("prompt missing preferred_language field:\n%s", prompt)
	}
}

// ---- titleJSONKeys (voice_engine.go) ----

func TestTitleJSONKeys_WithTitle(t *testing.T) {
	got := titleJSONKeys(true)
	if got != "title, " {
		t.Fatalf("titleJSONKeys(true) = %q, want 'title, '", got)
	}
}

func TestTitleJSONKeys_WithoutTitle(t *testing.T) {
	got := titleJSONKeys(false)
	if got != "" {
		t.Fatalf("titleJSONKeys(false) = %q, want ''", got)
	}
}

// ---- recurringPostKindSection (voice_engine.go) ----

func TestRecurringPostKindSection_Announcement(t *testing.T) {
	p := params{"recurring_post_kind": "announcement"}
	out := recurringPostKindSection(p)
	if !strings.Contains(strings.ToLower(out), "announcement") {
		t.Fatalf("expected announcement in section, got: %q", out)
	}
}

func TestRecurringPostKindSection_Main(t *testing.T) {
	p := params{"recurring_post_kind": "main"}
	out := recurringPostKindSection(p)
	if !strings.Contains(strings.ToLower(out), "main") {
		t.Fatalf("expected main event in section, got: %q", out)
	}
}

func TestRecurringPostKindSection_UnknownKindIsEmpty(t *testing.T) {
	p := params{"recurring_post_kind": "other"}
	if got := recurringPostKindSection(p); got != "" {
		t.Fatalf("expected empty string for unknown kind, got: %q", got)
	}
}

// ---- buildRefinePrompt (voice_engine.go) ----

func TestBuildRefinePrompt_ContainsPrimaryAccountID(t *testing.T) {
	ctx := testContext()
	selected := []domain.AIAccountSummary{
		{ID: "acc-1", Provider: "mastodon", Username: "main", MaxChars: 500},
	}
	p := params{
		"source_content": "Existing draft text here.",
		"prompt_hint":    "Make it punchier.",
	}
	prompt := buildRefinePrompt(ctx, p, selected, 500, "acc-1", nil, false)
	if !strings.Contains(prompt, "acc-1") {
		t.Fatalf("refine prompt missing primary account id:\n%s", prompt)
	}
	if !strings.Contains(prompt, "500") {
		t.Fatalf("refine prompt missing character limit:\n%s", prompt)
	}
}

func TestBuildRefinePrompt_WithTitle(t *testing.T) {
	ctx := testContext()
	selected := []domain.AIAccountSummary{
		{ID: "acc-1", Provider: "mastodon", Username: "main", MaxChars: 500},
	}
	p := params{
		"source_content": "Draft.",
		"title_hint":     "Use the event name as title.",
	}
	prompt := buildRefinePrompt(ctx, p, selected, 500, "acc-1", nil, true)
	if !strings.Contains(prompt, "title") {
		t.Fatalf("refine prompt with title should mention title:\n%s", prompt)
	}
}

// ---- Generate (llm.go) ----

func TestGenerate_DelegatesCompleteAndReturnsContent(t *testing.T) {
	client := &scriptedClient{responses: []Response{{Content: "hello from LLM"}}}
	got, err := Generate(context.Background(), client, "sys", "prompt", 0.5, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello from LLM" {
		t.Fatalf("Generate = %q, want 'hello from LLM'", got)
	}
	if len(client.requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(client.requests))
	}
	// Generate must NOT set the JSON flag.
	if client.requests[0].JSON {
		t.Fatal("Generate must not set JSON flag")
	}
}

// ---- apiError.Error (llm.go) ----

func TestAPIErrorMessage_FormatsStatusAndBody(t *testing.T) {
	err := &apiError{status: 429, body: "rate limited"}
	want := "llm api error: status 429: rate limited"
	if err.Error() != want {
		t.Fatalf("apiError.Error() = %q, want %q", err.Error(), want)
	}
}

// ---- observedClient.Model (observe.go) ----

func TestObservedClientModel_DelegatesToInner(t *testing.T) {
	inner := &scriptedClient{responses: []Response{{Content: "hi"}}}
	obs := observedClient{
		inner:    inner,
		obs:      &recordingObserver{},
		provider: ProviderOpenAI,
	}
	if got := obs.Model(); got != "scripted" {
		t.Fatalf("observedClient.Model() = %q, want 'scripted'", got)
	}
}

// ---- ResolvedModel (llm.go) ----

func TestResolvedModel_EmptyUsesOpenAIDefault(t *testing.T) {
	s := Settings{Provider: ProviderOpenAI, Model: ""}
	if got := s.ResolvedModel(); got != DefaultOpenAIModel {
		t.Fatalf("ResolvedModel = %q, want %q", got, DefaultOpenAIModel)
	}
}

func TestResolvedModel_EmptyAnthropicUsesAnthropicDefault(t *testing.T) {
	s := Settings{Provider: ProviderAnthropic, Model: ""}
	if got := s.ResolvedModel(); got != DefaultAnthropicModel {
		t.Fatalf("ResolvedModel = %q, want %q", got, DefaultAnthropicModel)
	}
}

func TestResolvedModel_ExplicitModelPreserved(t *testing.T) {
	s := Settings{Provider: ProviderOpenAI, Model: "gpt-5"}
	if got := s.ResolvedModel(); got != "gpt-5" {
		t.Fatalf("ResolvedModel = %q, want gpt-5", got)
	}
}
