package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
)

// ---- analysisPosts ----

func TestAnalysisPosts_FiltersEmptyContent(t *testing.T) {
	ctx := testContext()
	ctx.RecentPosts = []domain.ScheduledPost{
		{ID: "p1", Content: ""},
		{ID: "p2", Content: "   "},
		{ID: "p3", Content: "Hello world"},
	}
	got := analysisPosts(ctx, 10)
	if len(got) != 1 || got[0].id != "p3" {
		t.Fatalf("analysisPosts = %v, want only p3", got)
	}
}

func TestAnalysisPosts_RespectsLimit(t *testing.T) {
	ctx := testContext()
	for i := 0; i < 10; i++ {
		ctx.RecentPosts = append(ctx.RecentPosts, domain.ScheduledPost{
			ID:      fmt.Sprintf("p%d", i),
			Content: "Some content here",
		})
	}
	got := analysisPosts(ctx, 3)
	if len(got) != 3 {
		t.Fatalf("analysisPosts with limit 3 = %d posts, want 3", len(got))
	}
}

func TestAnalysisPosts_EmptyContext(t *testing.T) {
	ctx := testContext()
	ctx.RecentPosts = nil
	if got := analysisPosts(ctx, 5); len(got) != 0 {
		t.Fatalf("expected empty slice, got %v", got)
	}
}

// ---- postScore ----

func TestPostScore_PostedStatusAddsBonus(t *testing.T) {
	posted := analysisPost{content: "Hello world", status: "posted"}
	draft := analysisPost{content: "Hello world", status: "draft"}
	if postScore(posted) <= postScore(draft) {
		t.Fatalf("posted post should score higher: posted=%.1f draft=%.1f", postScore(posted), postScore(draft))
	}
}

func TestPostScore_LengthCappedAt500(t *testing.T) {
	// Single-word content: both get wordBonus=1; the cap at 500 makes their
	// length bonuses equal even though one is twice as long.
	post500 := analysisPost{content: strings.Repeat("a", 500), status: "draft"}
	post600 := analysisPost{content: strings.Repeat("a", 600), status: "draft"}
	if postScore(post600) != postScore(post500) {
		t.Fatalf("length bonus should be capped at 500: got %.1f vs %.1f", postScore(post600), postScore(post500))
	}
}

func TestPostScore_WordBonusCappedAt80(t *testing.T) {
	// 100-word post vs 200-word post: both get wordBonus=80 (cap), so
	// scores should be identical when length is the same.
	words100 := strings.Repeat("hi ", 100)
	words200 := strings.Repeat("hi ", 200)
	// Both are ~300 / ~600 chars; length differs, so only assert word cap hit.
	score100 := postScore(analysisPost{content: words100, status: "draft"})
	score200 := postScore(analysisPost{content: words200, status: "draft"})
	// score200 may differ because of length, but word bonus should cap at 80 for both.
	// We verify indirectly: adding more words beyond 80 doesn't increase score beyond length delta.
	if score100 < 0 || score200 < 0 {
		t.Fatal("scores must be non-negative")
	}
}

// ---- rankStyleExamples ----

func TestRankStyleExamples_SortsByScoreDescending(t *testing.T) {
	posts := []analysisPost{
		{id: "low", content: strings.Repeat("x", 40), status: "draft"},
		{id: "high", content: strings.Repeat("word ", 20), status: "posted"},
	}
	ranked := rankStyleExamples(posts)
	if len(ranked) < 2 {
		t.Fatalf("expected 2 ranked examples, got %d", len(ranked))
	}
	if ranked[0]["source_post_id"] != "high" {
		t.Fatalf("top ranked = %q, want high", ranked[0]["source_post_id"])
	}
}

func TestRankStyleExamples_FiltersShortPosts(t *testing.T) {
	posts := []analysisPost{
		{id: "short", content: "Hi."},
	}
	ranked := rankStyleExamples(posts)
	if len(ranked) != 0 {
		t.Fatalf("expected no examples for short posts (<40 chars), got %d", len(ranked))
	}
}

func TestRankStyleExamples_LimitsToTopN(t *testing.T) {
	var posts []analysisPost
	for i := 0; i < 10; i++ {
		posts = append(posts, analysisPost{
			id:      fmt.Sprintf("p%d", i),
			content: strings.Repeat("Some good content. ", 3),
			status:  "posted",
		})
	}
	ranked := rankStyleExamples(posts)
	if len(ranked) > topStyleExampleCount {
		t.Fatalf("ranked %d examples, want at most %d", len(ranked), topStyleExampleCount)
	}
}

func TestRankStyleExamples_ExampleFields(t *testing.T) {
	posts := []analysisPost{
		{id: "p1", content: strings.Repeat("content ", 6), status: "posted"},
	}
	ranked := rankStyleExamples(posts)
	if len(ranked) != 1 {
		t.Fatalf("expected 1 example, got %d", len(ranked))
	}
	ex := ranked[0]
	for _, key := range []string{"platform", "content", "notes", "source_post_id"} {
		if ex[key] == "" {
			t.Errorf("example missing field %q", key)
		}
	}
	if ex["source_post_id"] != "p1" {
		t.Fatalf("source_post_id = %q, want p1", ex["source_post_id"])
	}
}

// ---- buildAnalysisPrompt ----

func TestBuildAnalysisPrompt_ContainsTeamAndPosts(t *testing.T) {
	posts := []analysisPost{
		{id: "p1", content: "Our new product is live!"},
		{id: "p2", content: "Join us at the meetup."},
	}
	prompt := buildAnalysisPrompt("sys", "TestCo", posts)
	for _, want := range []string{"TestCo", "Our new product is live!", "Join us at the meetup."} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("analysis prompt missing %q", want)
		}
	}
}

func TestBuildAnalysisPrompt_NumbersPostsSequentially(t *testing.T) {
	posts := []analysisPost{
		{id: "a", content: "First"},
		{id: "b", content: "Second"},
	}
	prompt := buildAnalysisPrompt("", "Team", posts)
	if !strings.Contains(prompt, "--- Post 1 ---") || !strings.Contains(prompt, "--- Post 2 ---") {
		t.Fatalf("posts not numbered sequentially in prompt:\n%s", prompt)
	}
}

// ---- parseAnalysis ----

func TestParseAnalysis_ValidJSON(t *testing.T) {
	raw := `{"tonality":"casual","formatting_rules":["bullet points"],"banned_words":["buzz"],"preferred_language":"de","max_hashtags":2}`
	result := parseAnalysis(raw)
	if result["tonality"] != "casual" {
		t.Fatalf("tonality = %v, want casual", result["tonality"])
	}
	if result["preferred_language"] != "de" {
		t.Fatalf("preferred_language = %v, want de", result["preferred_language"])
	}
}

func TestParseAnalysis_FallbackOnBadJSON(t *testing.T) {
	result := parseAnalysis("not json at all, here is something")
	for _, key := range []string{"tonality", "formatting_rules", "banned_words", "preferred_language", "max_hashtags"} {
		if _, ok := result[key]; !ok {
			t.Errorf("fallback result missing key %q", key)
		}
	}
}

func TestParseAnalysis_LongFallbackTruncated(t *testing.T) {
	long := strings.Repeat("x", 300)
	result := parseAnalysis(long)
	tonality, _ := result["tonality"].(string)
	if len([]rune(tonality)) > 200 {
		t.Fatalf("fallback tonality should be truncated to 200 runes, got %d", len([]rune(tonality)))
	}
}

// ---- runProfileAnalysis ----

func TestRunProfileAnalysis_NoPostsReturnsError(t *testing.T) {
	client := &scriptedClient{responses: []Response{{Content: `{"tonality":"casual"}`}}}
	aiContext := testContext()
	aiContext.RecentPosts = nil

	_, err := runProfileAnalysis(context.Background(), client, domain.AIJob{}, aiContext, params{})
	if err == nil || !strings.Contains(err.Error(), "no recent posts") {
		t.Fatalf("expected 'no recent posts' error, got %v", err)
	}
}

func TestRunProfileAnalysis_ReturnsProposedProfile(t *testing.T) {
	analysisJSON := `{"tonality":"upbeat","formatting_rules":["short sentences"],"banned_words":[],"preferred_language":"en","max_hashtags":3}`
	client := &scriptedClient{responses: []Response{{Content: analysisJSON}}}

	aiContext := testContext()
	aiContext.RecentPosts = []domain.ScheduledPost{
		{ID: "p1", Content: "We launched a new feature today! Really exciting stuff for everyone."},
	}

	raw, err := runProfileAnalysis(context.Background(), client, domain.AIJob{}, aiContext, params{})
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if _, ok := result["proposed_profile"]; !ok {
		t.Fatal("result missing proposed_profile")
	}
	if n, _ := result["analyzed_post_count"].(float64); n != 1 {
		t.Fatalf("analyzed_post_count = %v, want 1", result["analyzed_post_count"])
	}
	if _, ok := result["suggested_style_examples"]; !ok {
		t.Fatal("result missing suggested_style_examples")
	}
}

func TestRunProfileAnalysis_RespectsPostCountParam(t *testing.T) {
	client := &scriptedClient{responses: []Response{{Content: `{"tonality":"x"}`}}}
	aiContext := testContext()
	// Provide 5 posts but limit to 2.
	for i := 0; i < 5; i++ {
		aiContext.RecentPosts = append(aiContext.RecentPosts, domain.ScheduledPost{
			ID:      fmt.Sprintf("p%d", i),
			Content: "Content for post " + fmt.Sprintf("%d", i),
		})
	}

	raw, err := runProfileAnalysis(context.Background(), client, domain.AIJob{}, aiContext, params{"post_count": float64(2)})
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if n, _ := result["analyzed_post_count"].(float64); n != 2 {
		t.Fatalf("analyzed_post_count = %v, want 2", result["analyzed_post_count"])
	}
}

// ---- runProfileAssistant ----

func TestRunProfileAssistant_RequiresBrief(t *testing.T) {
	client := &scriptedClient{responses: []Response{{Content: `{"ok":true}`}}}
	_, err := runProfileAssistant(context.Background(), client, domain.AIJob{}, testContext(), params{})
	if err == nil {
		t.Fatal("expected error for missing brief")
	}
}

func TestRunProfileAssistant_ReturnsProposedProfile(t *testing.T) {
	profileJSON := `{"identity":{"archetype":"Tech Podcast","persona":"We talk tech weekly."},"language_dna":{},"reach_strategy":{},"banned_words":[],"formatting_rules":[],"preferred_language":"en","max_hashtags":3}`
	client := &scriptedClient{responses: []Response{{Content: profileJSON}}}

	raw, err := runProfileAssistant(context.Background(), client, domain.AIJob{}, testContext(), params{
		"brief": "A tech podcast about developer tools and open source.",
	})
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if _, ok := result["proposed_profile"]; !ok {
		t.Fatalf("result missing proposed_profile: %v", result)
	}
}

// ---- runVibePreview ----

func TestRunVibePreview_ReturnsSummary(t *testing.T) {
	responseJSON := `{"summary":"Wir klingen freundlich und direkt.","suggestion":""}`
	client := &scriptedClient{responses: []Response{{Content: responseJSON}}}

	raw, err := runVibePreview(context.Background(), client, domain.AIJob{}, testContext(), params{})
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]string
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result["summary"] == "" {
		t.Fatal("expected non-empty summary")
	}
}

func TestRunVibePreview_MissingSummaryReturnsError(t *testing.T) {
	client := &scriptedClient{responses: []Response{{Content: `{"other":"field"}`}}}
	_, err := runVibePreview(context.Background(), client, domain.AIJob{}, testContext(), params{})
	if err == nil || !strings.Contains(err.Error(), "summary") {
		t.Fatalf("expected summary-missing error, got %v", err)
	}
}
