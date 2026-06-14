package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"git.f4mily.net/goloom/internal/domain"
)

const (
	topStyleExampleCount = 5
	minPostChars         = 40
)

var wordRe = regexp.MustCompile(`\w+`)

// runProfileAnalysis extracts the team's writing style from recent posts.
func runProfileAnalysis(ctx context.Context, client Client, job domain.AIJob, aiContext domain.AIContext, p params) (json.RawMessage, error) {
	postCount := p.intval("post_count", 20)

	teamName := orDefault(strings.TrimSpace(aiContext.Team.Name), "Unknown Team")
	recentPosts := analysisPosts(aiContext, postCount)
	if len(recentPosts) == 0 {
		return nil, fmt.Errorf("no recent posts available for analysis")
	}

	prompt := buildAnalysisPrompt(BuildSystemPrompt(aiContext), teamName, recentPosts)
	content, err := GenerateJSON(ctx, client, "You are a brand voice analyst. Extract the team's writing style from their recent posts.", prompt, 0.7, defaultMaxTokens)
	if err != nil {
		return nil, err
	}

	proposedProfile := parseAnalysis(content)
	suggested := rankStyleExamples(recentPosts)

	return json.Marshal(map[string]any{
		"proposed_profile":         proposedProfile,
		"suggested_style_examples": suggested,
		"analyzed_post_count":      len(recentPosts),
	})
}

type analysisPost struct {
	id      string
	content string
	status  string
}

func analysisPosts(aiContext domain.AIContext, count int) []analysisPost {
	var parsed []analysisPost
	for _, post := range aiContext.RecentPosts {
		content := strings.TrimSpace(post.Content)
		if content == "" {
			continue
		}
		parsed = append(parsed, analysisPost{id: post.ID, content: content, status: string(post.Status)})
		if len(parsed) >= count {
			break
		}
	}
	return parsed
}

func postScore(post analysisPost) float64 {
	words := len(wordRe.FindAllString(post.content, -1))
	length := len(post.content)
	if length > 500 {
		length = 500
	}
	lengthBonus := float64(length) / 10.0
	wordBonus := float64(words)
	if wordBonus > 80 {
		wordBonus = 80
	}
	statusBonus := 0.0
	if strings.EqualFold(post.status, "posted") {
		statusBonus = 20.0
	}
	return statusBonus + lengthBonus + wordBonus
}

func rankStyleExamples(posts []analysisPost) []map[string]string {
	ranked := make([]analysisPost, len(posts))
	copy(ranked, posts)
	sort.SliceStable(ranked, func(i, j int) bool { return postScore(ranked[i]) > postScore(ranked[j]) })

	examples := []map[string]string{}
	for _, post := range ranked {
		if len([]rune(post.content)) < minPostChars {
			continue
		}
		examples = append(examples, map[string]string{
			"platform":       "general",
			"content":        post.content,
			"notes":          "Suggested from published post analysis",
			"source_post_id": post.id,
		})
		if len(examples) >= topStyleExampleCount {
			break
		}
	}
	return examples
}

func buildAnalysisPrompt(systemPrompt, teamName string, posts []analysisPost) string {
	blocks := make([]string, 0, len(posts))
	for i, post := range posts {
		blocks = append(blocks, fmt.Sprintf("--- Post %d ---\n%s", i+1, post.content))
	}
	return systemPrompt + "\n\n" +
		fmt.Sprintf("Analyze the following recent posts from team %q and extract the team's writing style.\n\n", teamName) +
		"For each aspect below, provide your analysis in JSON format:\n\n" +
		"1. Tonality: What is the overall tone? (e.g., professional, casual, humorous, authoritative)\n" +
		"2. Formatting rules: What formatting patterns do you observe? (e.g., use of line breaks, emoji placement, capitalization style, sentence length)\n" +
		"3. Banned words: Are there any words or phrases the team avoids?\n" +
		"4. Preferred language: What language are posts primarily written in?\n" +
		"5. Max hashtags: How many hashtags are typically used per post?\n\n" +
		"Recent posts to analyze:\n" + strings.Join(blocks, "\n\n") + "\n\n" +
		"Respond with ONLY a valid JSON object using this exact structure (no markdown, no code fences):\n" +
		`{
  "tonality": "description of tonality",
  "formatting_rules": ["rule 1", "rule 2"],
  "banned_words": ["word 1"],
  "preferred_language": "en",
  "max_hashtags": 3
}`
}

func parseAnalysis(raw string) map[string]any {
	payload, err := extractJSONObject(raw)
	if err != nil {
		tonality := strings.TrimSpace(raw)
		if runes := []rune(tonality); len(runes) > 200 {
			tonality = string(runes[:200])
		}
		return map[string]any{
			"tonality":           tonality,
			"formatting_rules":   []string{},
			"banned_words":       []string{},
			"preferred_language": "en",
			"max_hashtags":       3,
		}
	}
	return payload
}

// runProfileAssistant proposes a brand profile from a short user brief.
func runProfileAssistant(ctx context.Context, client Client, job domain.AIJob, aiContext domain.AIContext, p params) (json.RawMessage, error) {
	prompt, err := buildProfileAssistantPrompt(p)
	if err != nil {
		return nil, err
	}
	systemPrompt := "You are a senior social media strategist. You write brand profiles " +
		"that sound like the actual person or team, not like AI marketing copy."
	content, err := GenerateJSON(ctx, client, systemPrompt, prompt, 0.6, defaultMaxTokens)
	if err != nil {
		return nil, err
	}
	proposal, err := extractJSONObject(content)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"proposed_profile": proposal})
}

// runVibePreview summarizes the configured brand voice in one or two sentences.
func runVibePreview(ctx context.Context, client Client, job domain.AIJob, aiContext domain.AIContext, p params) (json.RawMessage, error) {
	prompt := buildVibePreviewPrompt(aiContext)
	content, err := GenerateJSON(ctx, client, "You summarize brand voice profiles concisely.", prompt, 0.5, 600)
	if err != nil {
		return nil, err
	}
	payload, err := extractJSONObject(content)
	if err != nil {
		return nil, err
	}
	summary := payloadString(payload, "summary")
	if summary == "" {
		return nil, fmt.Errorf("LLM response missing summary")
	}
	return json.Marshal(map[string]string{
		"summary":    summary,
		"suggestion": payloadString(payload, "suggestion"),
	})
}
