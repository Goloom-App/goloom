package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

var dayOffsetRe = regexp.MustCompile(`\{day([+-]\d+)\}`)
var monthOffsetRe = regexp.MustCompile(`\{month([+-]\d+)\}`)

type campaignResult struct {
	Content              string   `json:"content"`
	Hashtags             []string `json:"hashtags"`
	SuggestedScheduledAt string   `json:"suggested_scheduled_at,omitempty"`
}

// runCampaignAutopilot generates a post for a configured campaign format.
func runCampaignAutopilot(ctx context.Context, client Client, job domain.AIJob, aiContext domain.AIContext, p params) (json.RawMessage, error) {
	campaignFormatID := p.str("campaign_format_id")
	if campaignFormatID == "" {
		return nil, fmt.Errorf("campaign_format_id is required")
	}
	campaignFormat, err := findCampaignFormat(aiContext, campaignFormatID)
	if err != nil {
		return nil, err
	}

	platform := orDefault(p.str("platform"), "mastodon")
	constraints := applyPlatformConstraints(platform, 0)
	suggestedScheduledAt := resolveScheduledAt(p, campaignFormat, aiContext)
	renderedTemplate := renderStructureTemplate(decodeStructure(campaignFormat.Structure), p, suggestedScheduledAt)
	requiredHashtags := nonEmptyStrings(campaignFormat.RequiredHashtags)

	systemPrompt := BuildSystemPrompt(aiContext)
	basePrompt := buildGenerationPrompt(aiContext, campaignPromptParams(p, campaignFormat, renderedTemplate, requiredHashtags, suggestedScheduledAt), platform)

	prompt := basePrompt
	model := client.Model()
	maxTokens := modelBudgets.starting(model)
	var lastErr error
	for try := 0; try <= campaignMaxRetries; try++ {
		content, err := GenerateJSON(ctx, client, systemPrompt, prompt, 0.7, maxTokens)
		if err != nil {
			// Truncation needs a bigger budget, not a reworded prompt: note it for
			// the per-model memory and double the budget to recover this job now.
			if errors.Is(err, ErrResponseTruncated) && try < campaignMaxRetries {
				modelBudgets.learnTruncation(model)
				maxTokens = escalateBudget(maxTokens)
				lastErr = err
				continue
			}
			return nil, err
		}
		result, err := validateCampaignResult(content, requiredHashtags, constraints.charLimit, suggestedScheduledAt)
		if err == nil {
			return json.Marshal(result)
		}
		// The reply parsed or validated badly; re-prompt with the concrete defect
		// instead of failing the whole job on a single noisy response.
		lastErr = err
		prompt = basePrompt + "\n\nYour previous answer could not be used: " + err.Error() +
			". Respond with ONLY a valid JSON object with the keys \"content\" and \"hashtags\" — no prose, no markdown, no code fences."
	}
	return nil, lastErr
}

const campaignMaxRetries = 2

func findCampaignFormat(aiContext domain.AIContext, campaignFormatID string) (*domain.CampaignFormat, error) {
	for _, item := range aiContext.CampaignFormats {
		if item.ID == campaignFormatID {
			if !item.IsActive {
				return nil, fmt.Errorf("campaign format %s is inactive", campaignFormatID)
			}
			format := item
			return &format, nil
		}
	}
	return nil, fmt.Errorf("campaign format %s not found", campaignFormatID)
}

func campaignPromptParams(p params, campaignFormat *domain.CampaignFormat, renderedTemplate any, requiredHashtags []string, suggestedScheduledAt *time.Time) params {
	scheduleText := formatDatetime(suggestedScheduledAt)
	if scheduleText == "" {
		scheduleText = "unscheduled"
	}
	hashtagsText := "none"
	if len(requiredHashtags) > 0 {
		hashtagsText = strings.Join(requiredHashtags, ", ")
	}
	formatName := strings.TrimSpace(campaignFormat.Name)
	if formatName == "" {
		formatName = "unnamed format"
	}
	promptHint := fmt.Sprintf(
		"Generate a campaign auto-pilot post for the format '%s'.\n"+
			"Use this rendered structure template as the blueprint: %s\n"+
			"Required hashtags: %s. Every required hashtag must appear in the content.\n"+
			"Suggested schedule: %s.\n"+
			"Return JSON only with keys content and hashtags.",
		formatName, formatValue(renderedTemplate), hashtagsText, scheduleText,
	)

	out := params{}
	for key, value := range p {
		if key == "prompt_hint" {
			continue
		}
		out[key] = value
	}
	out["prompt_hint"] = promptHint
	out["campaign_format_name"] = campaignFormat.Name
	out["campaign_format_template"] = renderedTemplate
	out["required_hashtags"] = requiredHashtags
	out["suggested_scheduled_at"] = formatDatetime(suggestedScheduledAt)
	return out
}

func validateCampaignResult(rawContent string, requiredHashtags []string, charLimit int, suggestedScheduledAt *time.Time) (campaignResult, error) {
	payload, err := extractJSONObject(rawContent)
	if err != nil {
		return campaignResult{}, err
	}
	content := payloadString(payload, "content")
	if content == "" {
		return campaignResult{}, fmt.Errorf("LLM response missing content")
	}
	hashtags := payloadStringList(payload, "hashtags")

	lowerContent := strings.ToLower(content)
	var missing []string
	for _, tag := range requiredHashtags {
		if !strings.Contains(lowerContent, strings.ToLower(tag)) {
			missing = append(missing, tag)
		}
	}
	if len(missing) > 0 {
		content = strings.TrimSpace(content + " " + strings.Join(missing, " "))
		lowerContent = strings.ToLower(content)
	}
	for _, tag := range requiredHashtags {
		found := false
		for _, existing := range hashtags {
			if existing == tag {
				found = true
				break
			}
		}
		if !found {
			hashtags = append(hashtags, tag)
		}
	}

	if len([]rune(content)) > charLimit {
		return campaignResult{}, fmt.Errorf("generated content exceeds platform limit of %d characters", charLimit)
	}
	for _, tag := range requiredHashtags {
		if !strings.Contains(lowerContent, strings.ToLower(tag)) {
			return campaignResult{}, fmt.Errorf("generated content is missing required hashtags: %s", tag)
		}
	}

	return campaignResult{
		Content:              content,
		Hashtags:             orEmptyList(hashtags),
		SuggestedScheduledAt: formatDatetime(suggestedScheduledAt),
	}, nil
}

func decodeStructure(raw json.RawMessage) any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil || decoded == nil {
		return map[string]any{}
	}
	return decoded
}

func renderStructureTemplate(template any, p params, scheduledAt *time.Time) any {
	switch v := template.(type) {
	case map[string]any:
		out := map[string]any{}
		for key, value := range v {
			out[key] = renderStructureTemplate(value, p, scheduledAt)
		}
		return out
	case []any:
		out := make([]any, 0, len(v))
		for _, value := range v {
			out = append(out, renderStructureTemplate(value, p, scheduledAt))
		}
		return out
	case string:
		return renderTextTemplate(v, p, scheduledAt)
	default:
		return template
	}
}

func renderTextTemplate(text string, p params, scheduledAt *time.Time) string {
	if text == "" {
		return ""
	}

	basis := nowFunc()
	if scheduledAt != nil {
		basis = scheduledAt.UTC()
	}

	rendered := dayOffsetRe.ReplaceAllStringFunc(text, func(match string) string {
		offset, _ := strconv.Atoi(dayOffsetRe.FindStringSubmatch(match)[1])
		return zeroPad(clampDay(basis.Day() + offset))
	})
	rendered = monthOffsetRe.ReplaceAllStringFunc(rendered, func(match string) string {
		offset, _ := strconv.Atoi(monthOffsetRe.FindStringSubmatch(match)[1])
		return zeroPad(clampMonth(int(basis.Month()) + offset))
	})

	replacements := map[string]string{
		"{year}":              strconv.Itoa(basis.Year()),
		"{month}":             zeroPad(int(basis.Month())),
		"{day}":               zeroPad(basis.Day()),
		"{weekday}":           strconv.Itoa(int(basis.Weekday())),
		"{weekday_name}":      weekdayNamesEN[int(basis.Weekday())],
		"{main_day}":          "",
		"{main_month}":        "",
		"{main_weekday_name}": "",
		"{campaign_name}":     p.str("campaign_name", "name"),
		"{format_name}":       p.str("format_name"),
	}
	for key, value := range replacements {
		rendered = strings.ReplaceAll(rendered, key, value)
	}

	for key, value := range p {
		rendered = strings.ReplaceAll(rendered, "{"+key+"}", formatValue(value))
	}
	return rendered
}

func zeroPad(value int) string {
	return fmt.Sprintf("%02d", value)
}

func clampDay(value int) int {
	if value < 1 {
		return 1
	}
	if value > 31 {
		return 31
	}
	return value
}

func clampMonth(value int) int {
	if value < 1 {
		return 1
	}
	if value > 12 {
		return 12
	}
	return value
}
