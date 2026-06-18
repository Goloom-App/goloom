package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

const voiceEngineMaxRetries = 3

type voiceEngineResult struct {
	Title                  string            `json:"title,omitempty"`
	Content                string            `json:"content"`
	Hashtags               []string          `json:"hashtags"`
	PlatformMetadata       map[string]any    `json:"platform_metadata"`
	AccountContentOverride map[string]string `json:"account_content_override"`
	ScheduledAt            string            `json:"scheduled_at,omitempty"`
	PrimaryAccountID       string            `json:"primary_account_id"`
	// InjectionWarning lists prompt-injection markers detected in the external
	// source material, surfaced so the UI can flag the draft for review. Empty
	// when none were found.
	InjectionWarning []string `json:"injection_warning,omitempty"`
	// PIIWarning lists the kinds of PII redacted from the user-supplied request
	// before it reached the model. Empty when none were found.
	PIIWarning []string `json:"pii_warning,omitempty"`
}

type parsedVoiceResult struct {
	title            string
	content          string
	hashtags         []string
	platformMetadata map[string]any
	overrides        map[string]string
}

// runVoiceEngine generates a multi-account post draft in the team's voice.
func runVoiceEngine(ctx context.Context, client Client, job domain.AIJob, aiContext domain.AIContext, p params) (json.RawMessage, error) {
	selected, err := selectedAccounts(aiContext, p)
	if err != nil {
		return nil, err
	}

	campaignFormat, err := optionalCampaignFormat(aiContext, p)
	if err != nil {
		return nil, err
	}

	var scheduledAt *time.Time
	if schedule, ok := p["schedule"].(bool); !ok || schedule {
		scheduledAt = resolveScheduledAt(p, campaignFormat, aiContext)
	}

	primary := primaryAccount(selected)
	primaryLimit := primary.MaxChars
	primaryAccountID := primary.ID

	piiKinds := redactParamsPII(p)
	if len(piiKinds) > 0 {
		slog.Warn("ai request contained PII that was redacted before the model call",
			"team", aiContext.Team.ID, "job_type", job.Type, "kinds", piiKinds)
	}

	injectionHits := scanParamsForInjection(p)
	if len(injectionHits) > 0 {
		slog.Warn("ai source material contains possible prompt injection",
			"team", aiContext.Team.ID, "job_type", job.Type, "markers", injectionHits)
	}

	systemPrompt := BuildSystemPrompt(aiContext)
	refineMode := isRefineMode(p)
	recurringGeneration := isRecurringKind(recurringPostKind(p))
	includeTitle := includeTitleInResponse(p, refineMode)

	var prompt string
	if refineMode {
		prompt = buildRefinePrompt(aiContext, p, selected, primaryLimit, primaryAccountID, scheduledAt, includeTitle)
	} else {
		prompt = buildMultiAccountPrompt(aiContext, p, selected, primaryLimit, scheduledAt, includeTitle)
	}

	parsed, err := generateWithRetries(ctx, client, generateAttempt{
		prompt:              prompt,
		systemPrompt:        systemPrompt,
		primaryLimit:        primaryLimit,
		primaryAccountID:    primaryAccountID,
		selectedAccounts:    selected,
		refineMode:          refineMode,
		recurringGeneration: recurringGeneration,
		includeTitle:        includeTitle,
	})
	if err != nil {
		return nil, err
	}

	normalizeMultiAccountResult(&parsed, selected, primaryAccountID, primaryLimit)
	mergeHashtagsIntoContent(&parsed, maxHashtagsFromContext(aiContext), primaryLimit)

	result := voiceEngineResult{
		Title:                  parsed.title,
		Content:                parsed.content,
		Hashtags:               orEmptyList(parsed.hashtags),
		PlatformMetadata:       parsed.platformMetadata,
		AccountContentOverride: parsed.overrides,
		ScheduledAt:            formatDatetime(scheduledAt),
		PrimaryAccountID:       primaryAccountID,
		InjectionWarning:       injectionHits,
		PIIWarning:             piiKinds,
	}
	if result.PlatformMetadata == nil {
		result.PlatformMetadata = map[string]any{}
	}
	if result.AccountContentOverride == nil {
		result.AccountContentOverride = map[string]string{}
	}
	return json.Marshal(result)
}

func orEmptyList(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func selectedAccounts(aiContext domain.AIContext, p params) ([]domain.AIAccountSummary, error) {
	rawIDs := p.stringList("target_account_ids", "targetAccountIds")
	wanted := map[string]bool{}
	for _, id := range rawIDs {
		wanted[id] = true
	}
	if len(wanted) == 0 {
		return nil, fmt.Errorf("target_account_ids must include at least one account")
	}
	var selected []domain.AIAccountSummary
	for _, account := range aiContext.Accounts {
		if wanted[account.ID] {
			selected = append(selected, account)
		}
	}
	if len(selected) != len(wanted) {
		return nil, fmt.Errorf("one or more target accounts were not found in team context")
	}
	return selected, nil
}

func primaryAccount(selected []domain.AIAccountSummary) domain.AIAccountSummary {
	primary := selected[0]
	for _, account := range selected[1:] {
		if account.MaxChars > primary.MaxChars {
			primary = account
		}
	}
	return primary
}

func optionalCampaignFormat(aiContext domain.AIContext, p params) (*domain.CampaignFormat, error) {
	campaignFormatID := p.str("campaign_format_id", "campaignFormatId")
	if campaignFormatID == "" {
		return nil, nil
	}
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

func isRefineMode(p params) bool {
	explicitRefine := p.boolean("refine_content") || p.boolean("refine")
	if isRecurringKind(recurringPostKind(p)) {
		return explicitRefine
	}
	if p.nested("rss_automation") != nil || p.str("rss_article_title") != "" {
		return explicitRefine
	}
	if explicitRefine {
		return true
	}
	return p.str("source_content", "existing_content") != ""
}

func includeTitleInResponse(p params, refineMode bool) bool {
	if isRecurringKind(recurringPostKind(p)) {
		return p.str("title_hint") != ""
	}
	if !refineMode {
		return true
	}
	return p.str("title_hint") != ""
}

func titleJSONKeys(includeTitle bool) string {
	if includeTitle {
		return "title, "
	}
	return ""
}

func titleJSONInstruction(p params, includeTitle bool) string {
	if !includeTitle {
		return ""
	}
	if hint := p.str("title_hint"); hint != "" {
		return fmt.Sprintf("- \"title\": internal Goloom post title (max 120 characters). Instruction: %s\n", hint)
	}
	return "- \"title\": short internal Goloom post title (max 120 characters) for editors, based on the post content\n"
}

func recurringPostKindSection(p params) string {
	switch strings.ToLower(p.str("recurring_post_kind")) {
	case "announcement":
		return "Recurring ANNOUNCEMENT — the rendered template shows the intended pre-event wording. " +
			"Do not replace a named date with “heute”.\n"
	case "main":
		return "Recurring MAIN EVENT — the rendered template shows the intended event-day wording. " +
			"Keep “heute”/“today” if the template uses it. Stay consistent with any announcement reference.\n"
	}
	return ""
}

func recurringGenerationRules(p params) string {
	if !isRecurringKind(recurringPostKind(p)) {
		return ""
	}
	return "Recurring post rules:\n" +
		"- Fresh, engaging copy grounded in the template and editorial direction.\n" +
		"- Keep timing, links, numbers, and names from the grounding sources.\n" +
		"- Do not invent factual details; persuasive emphasis on stated facts is welcome.\n" +
		"- Brand knowledge and recent posts are for tone only — not new topics.\n\n"
}

func rssGenerationRules(p params) string {
	if p.nested("rss_automation") == nil && p.str("rss_article_title") == "" {
		return ""
	}
	return "RSS item rules:\n" +
		"- This is a NEW post written from the feed item body, not a polish of a previous draft.\n" +
		"- Keep the title and any numbers/identifiers exactly as in the source.\n" +
		"- Use the item link from the source — never substitute the RSS feed subscription URL.\n" +
		"- Include at least two concrete details from the body; skip generic filler not supported by the source.\n\n"
}

func accountLines(selected []domain.AIAccountSummary) string {
	lines := make([]string, 0, len(selected))
	for _, account := range selected {
		name := account.Username
		if name == "" {
			name = account.ID
		}
		lines = append(lines, fmt.Sprintf("- %s (id=%s, %s): max %d characters", name, account.ID, account.Provider, account.MaxChars))
	}
	return strings.Join(lines, "\n")
}

func lowerLimitAccounts(selected []domain.AIAccountSummary, primaryLimit int) []domain.AIAccountSummary {
	var lower []domain.AIAccountSummary
	for _, account := range selected {
		if account.MaxChars < primaryLimit {
			lower = append(lower, account)
		}
	}
	return lower
}

func overrideHintFor(lower []domain.AIAccountSummary, emptyHint string) string {
	if len(lower) == 0 {
		return emptyHint
	}
	parts := make([]string, 0, len(lower))
	for _, account := range lower {
		name := account.Username
		if name == "" {
			name = account.ID
		}
		parts = append(parts, fmt.Sprintf("%s (id=%s, max %d)", name, account.ID, account.MaxChars))
	}
	return "account_content_override must ONLY contain compressed variants for these lower-limit accounts " +
		"when the primary text would exceed their limit: " + strings.Join(parts, ", ")
}

func buildMultiAccountPrompt(aiContext domain.AIContext, p params, selected []domain.AIAccountSummary, primaryLimit int, scheduledAt *time.Time, includeTitle bool) string {
	primary := primaryAccount(selected)
	primaryPlatform := orDefault(primary.Provider, "general")
	basePrompt := buildGenerationPrompt(aiContext, p, primaryPlatform)

	lower := lowerLimitAccounts(selected, primaryLimit)
	overrideHint := overrideHintFor(lower, "No account_content_override entries are needed because every selected account shares the same limit.")

	scheduleLabel := formatDatetime(scheduledAt)
	if scheduleLabel == "" {
		scheduleLabel = "next available slot"
	}
	primaryName := primary.Username
	if primaryName == "" {
		primaryName = primary.ID
	}

	return basePrompt + "\n\n" +
		rssGenerationRules(p) +
		recurringGenerationRules(p) +
		"Multi-account output rules:\n" +
		fmt.Sprintf("- Primary account: %s (id=%s, %s, limit %d characters).\n", primaryName, primary.ID, primaryPlatform, primaryLimit) +
		fmt.Sprintf("- Write \"content\" ONLY for the primary account. Make it as long and complete as possible, targeting roughly %d to %d characters.\n", primaryLimit-20, primaryLimit) +
		"- " + overrideHint + "\n" +
		"- Do NOT create a separate version for every account.\n" +
		"- Accounts with the same or higher limit than the primary use \"content\" unchanged.\n" +
		"- Overrides must be shorter compressions of the same message, not alternate drafts.\n" +
		"Return JSON only with keys:\n" +
		titleJSONInstruction(p, includeTitle) +
		fmt.Sprintf("- \"content\": primary text for account id %s\n", primary.ID) +
		"- \"account_content_override\": object mapping account_id -> shorter text ONLY where required\n" +
		"- \"hashtags\": array of hashtags\n" +
		"- \"platform_metadata\": object\n" +
		"Accounts:\n" + accountLines(selected) +
		"\nTarget schedule (UTC): " + scheduleLabel + "."
}

func buildRefinePrompt(aiContext domain.AIContext, p params, selected []domain.AIAccountSummary, primaryLimit int, primaryAccountID string, scheduledAt *time.Time, includeTitle bool) string {
	primary := primaryAccount(selected)
	primaryPlatform := orDefault(primary.Provider, "general")
	sourceContent := p.str("source_content", "existing_content")
	refinementHint := p.str("prompt_hint", "instruction")
	if refinementHint == "" {
		refinementHint = "Improve clarity, flow, and engagement while preserving the core message and team voice."
	}

	refineParams := params{}
	for key, value := range p {
		refineParams[key] = value
	}
	refineParams["prompt_hint"] = refinementHint
	basePrompt := buildGenerationPrompt(aiContext, refineParams, primaryPlatform)

	lower := lowerLimitAccounts(selected, primaryLimit)
	overrideHint := overrideHintFor(lower, "No account_content_override entries are needed when the refined primary text fits every account.")

	scheduleLabel := formatDatetime(scheduledAt)
	if scheduleLabel == "" {
		scheduleLabel = "unchanged"
	}
	primaryName := primary.Username
	if primaryName == "" {
		primaryName = primaryAccountID
	}

	refineIntro := "Refine an existing draft for multi-account publishing.\n"
	if isRecurringKind(strings.ToLower(p.str("recurring_post_kind"))) {
		refineIntro = "Improve the rendered recurring template for multi-account publishing. " +
			"Preserve its date/time wording (named date vs. heute/today).\n"
	}

	return basePrompt + "\n\n" +
		refineIntro +
		recurringPostKindSection(p) +
		fmt.Sprintf("Primary account: %s (id=%s, limit %d characters).\n", primaryName, primaryAccountID, primaryLimit) +
		"Refinement goal: " + refinementHint + "\n" +
		fmt.Sprintf("- \"content\": refined primary text for account id %s; MUST NOT exceed %d characters (hard limit)\n", primaryAccountID, primaryLimit) +
		fmt.Sprintf("- The source draft is %d characters; keep the refined text within %d\n", len([]rune(sourceContent)), primaryLimit) +
		"- " + overrideHint + "\n" +
		titleJSONInstruction(p, includeTitle) +
		"- Do NOT create a separate version for every account.\n" +
		"- Overrides must be shorter compressions of the refined primary text.\n" +
		fmt.Sprintf("Return JSON only with keys %scontent, account_content_override, hashtags, platform_metadata.\n", titleJSONKeys(includeTitle)) +
		"Accounts:\n" + accountLines(selected) +
		"\nTarget schedule (UTC): " + scheduleLabel + "."
}

type generateAttempt struct {
	prompt              string
	systemPrompt        string
	primaryLimit        int
	primaryAccountID    string
	selectedAccounts    []domain.AIAccountSummary
	refineMode          bool
	recurringGeneration bool
	includeTitle        bool
}

func generateWithRetries(ctx context.Context, client Client, attempt generateAttempt) (parsedVoiceResult, error) {
	currentPrompt := attempt.prompt
	lastError := "invalid multi-account output"
	model := client.Model()
	maxTokens := modelBudgets.starting(model)
	for try := 0; try <= voiceEngineMaxRetries; try++ {
		content, err := GenerateJSON(ctx, client, attempt.systemPrompt, currentPrompt, 0.7, maxTokens)
		if err != nil {
			// Truncation means the model needs more room, not a different prompt:
			// note it for the per-model memory and double the budget to recover
			// this job now (cross-job learning is gentle; in-job recovery is not).
			if errors.Is(err, ErrResponseTruncated) && try < voiceEngineMaxRetries {
				modelBudgets.learnTruncation(model)
				maxTokens = escalateBudget(maxTokens)
				continue
			}
			return parsedVoiceResult{}, err
		}
		result, parseErr := parseVoiceResult(content, attempt.includeTitle, attempt.refineMode)
		if parseErr == nil {
			normalizeMultiAccountResult(&result, attempt.selectedAccounts, attempt.primaryAccountID, attempt.primaryLimit)
			// Final attempt: rather than fail the whole job because the model never
			// produced a required per-account override, derive the missing ones by
			// compressing the primary text. Earlier attempts still push the model to
			// craft a better override via the retry feedback below.
			if try >= voiceEngineMaxRetries {
				fillMissingOverrides(&result, attempt)
			}
			parseErr = validateLengths(result, attempt)
			if parseErr == nil {
				return result, nil
			}
		}
		lastError = parseErr.Error()
		if try >= voiceEngineMaxRetries {
			return parsedVoiceResult{}, parseErr
		}

		switch {
		case attempt.refineMode:
			currentPrompt = attempt.prompt + "\n\nRevise the previous answer and return JSON only. " +
				"Fix this issue: " + lastError + ". " +
				fmt.Sprintf("The primary content MUST be at most %d characters. ", attempt.primaryLimit) +
				"Keep the refined primary text faithful to the source draft while improving quality. " +
				"Only add account_content_override entries for lower-limit accounts that cannot fit the primary text."
		case attempt.recurringGeneration:
			currentPrompt = attempt.prompt + "\n\nRevise the previous answer and return JSON only. " +
				"Fix this issue: " + lastError + ". " +
				fmt.Sprintf("The primary content MUST be at most %d characters. ", attempt.primaryLimit) +
				"Write fresh, grounded copy — emphasize stated facts, do not invent new ones or paste the template verbatim. " +
				"Only add account_content_override entries for lower-limit accounts that cannot fit the primary text."
		default:
			currentPrompt = attempt.prompt + "\n\nRevise the previous answer and return JSON only. " +
				"Fix this issue: " + lastError + ". " +
				fmt.Sprintf("The primary content for account %s must be long (target %d to %d characters). ",
					attempt.primaryAccountID, minPrimaryLength(attempt.primaryLimit, attempt.selectedAccounts), attempt.primaryLimit) +
				"Only add account_content_override entries for lower-limit accounts that cannot fit the primary text."
		}
	}
	return parsedVoiceResult{}, fmt.Errorf("voice engine retry loop exited unexpectedly")
}

func minPrimaryLength(primaryLimit int, selected []domain.AIAccountSummary) int {
	base := primaryLimit - 20
	if scaled := primaryLimit * 85 / 100; scaled < base {
		base = scaled
	}
	maxLower := 0
	for _, account := range selected {
		if account.MaxChars < primaryLimit && account.MaxChars > maxLower {
			maxLower = account.MaxChars
		}
	}
	if maxLower > 0 {
		if base < maxLower+1 {
			return maxLower + 1
		}
		return base
	}
	if floor := primaryLimit * 70 / 100; base < floor {
		return floor
	}
	return base
}

func truncateToLimit(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	cleaned := strings.TrimSpace(text)
	runes := []rune(cleaned)
	if len(runes) <= limit {
		return cleaned
	}
	if limit == 1 {
		return string(runes[:1])
	}
	clipped := runes[:limit]
	window := limit / 6
	if window < 20 {
		window = 20
	}
	windowStart := limit - window
	if windowStart < 0 {
		windowStart = 0
	}
	slice := string(clipped[windowStart:])
	if lastSpace := strings.LastIndex(slice, " "); lastSpace > 0 {
		return strings.TrimRight(string(clipped[:windowStart])+slice[:lastSpace], " ")
	}
	return strings.TrimRight(string(clipped), " ")
}

func normalizeMultiAccountResult(result *parsedVoiceResult, selected []domain.AIAccountSummary, primaryAccountID string, primaryLimit int) {
	content := result.content
	if len([]rune(content)) > primaryLimit {
		content = truncateToLimit(content, primaryLimit)
	}
	limits := map[string]int{}
	for _, account := range selected {
		limits[account.ID] = account.MaxChars
	}

	overrides := map[string]string{}
	for accountID, value := range result.overrides {
		if accountID == primaryAccountID {
			continue
		}
		limit, ok := limits[accountID]
		if !ok || limit >= primaryLimit {
			continue
		}
		text := strings.TrimSpace(value)
		if text == "" {
			continue
		}
		if len([]rune(content)) <= limit {
			continue
		}
		if len([]rune(text)) > limit {
			text = truncateToLimit(text, limit)
		}
		if len([]rune(text)) >= len([]rune(content)) {
			continue
		}
		overrides[accountID] = text
	}

	result.content = content
	result.overrides = overrides
}

// fillMissingOverrides is the final-attempt safety net: for every lower-limit
// account whose limit the primary content exceeds but for which the model
// supplied no override, it derives one by truncating the primary text to fit.
// This guarantees a usable multi-account result instead of failing the job.
func fillMissingOverrides(result *parsedVoiceResult, attempt generateAttempt) {
	content := result.content
	contentLen := len([]rune(content))
	for _, account := range attempt.selectedAccounts {
		if account.ID == attempt.primaryAccountID || account.MaxChars <= 0 {
			continue
		}
		if contentLen <= account.MaxChars {
			continue
		}
		if existing, ok := result.overrides[account.ID]; ok && strings.TrimSpace(existing) != "" {
			continue
		}
		if result.overrides == nil {
			result.overrides = map[string]string{}
		}
		result.overrides[account.ID] = truncateToLimit(content, account.MaxChars)
	}
}

func validateLengths(result parsedVoiceResult, attempt generateAttempt) error {
	content := result.content
	contentLen := len([]rune(content))
	skipMin := attempt.refineMode || attempt.recurringGeneration
	if !attempt.refineMode && !skipMin {
		minPrimary := minPrimaryLength(attempt.primaryLimit, attempt.selectedAccounts)
		if contentLen < minPrimary {
			return fmt.Errorf("Primary content is too short (%d chars); aim for at least %d", contentLen, minPrimary)
		}
	}
	if contentLen < 1 {
		return fmt.Errorf("Primary content is empty")
	}
	if contentLen > attempt.primaryLimit {
		return fmt.Errorf("Generated primary content exceeds limit of %d characters", attempt.primaryLimit)
	}

	if _, exists := result.overrides[attempt.primaryAccountID]; exists {
		return fmt.Errorf("Primary account must not appear in account_content_override")
	}

	for _, account := range attempt.selectedAccounts {
		if account.ID == attempt.primaryAccountID {
			continue
		}
		override, hasOverride := result.overrides[account.ID]
		if contentLen <= account.MaxChars {
			if hasOverride {
				return fmt.Errorf("Remove override for account %s; primary content already fits", account.ID)
			}
			continue
		}
		if !hasOverride {
			return fmt.Errorf("Missing override for account %s with limit %d", account.ID, account.MaxChars)
		}
		overrideLen := len([]rune(override))
		if overrideLen > account.MaxChars {
			return fmt.Errorf("Override for account %s exceeds limit of %d", account.ID, account.MaxChars)
		}
		if overrideLen >= contentLen {
			return fmt.Errorf("Override for account %s must be shorter than the primary content", account.ID)
		}
	}
	return nil
}

func maxHashtagsFromContext(aiContext domain.AIContext) int {
	maxHashtags := styleMetadata(aiContext).MaxHashtags
	if maxHashtags < 0 {
		return 0
	}
	return maxHashtags
}

func normalizeHashtag(tag string) string {
	cleaned := strings.TrimSpace(tag)
	if cleaned == "" {
		return ""
	}
	if strings.HasPrefix(cleaned, "#") {
		return cleaned
	}
	return "#" + strings.TrimLeft(cleaned, "#")
}

func mergeHashtagsIntoContent(result *parsedVoiceResult, maxHashtags, charLimit int) {
	content := strings.TrimSpace(result.content)

	var normalized []string
	seen := map[string]bool{}
	for _, tag := range result.hashtags {
		normalizedTag := normalizeHashtag(tag)
		if normalizedTag == "" {
			continue
		}
		key := strings.ToLower(normalizedTag)
		if seen[key] {
			continue
		}
		seen[key] = true
		normalized = append(normalized, normalizedTag)
	}
	if maxHashtags > 0 && len(normalized) > maxHashtags {
		normalized = normalized[:maxHashtags]
	}

	lowerContent := strings.ToLower(content)
	for _, tag := range normalized {
		if strings.Contains(lowerContent, strings.ToLower(tag)) {
			continue
		}
		candidate := strings.TrimSpace(content + " " + tag)
		if len([]rune(candidate)) <= charLimit {
			content = candidate
			lowerContent = strings.ToLower(content)
		} else {
			break
		}
	}

	result.content = content
	result.hashtags = normalized
}

func coerceAccountContentOverride(value any) map[string]string {
	overrides := map[string]string{}
	switch v := value.(type) {
	case nil:
		return overrides
	case map[string]any:
		for key, item := range v {
			text := strings.TrimSpace(fmt.Sprintf("%v", item))
			if strings.TrimSpace(key) != "" && text != "" {
				overrides[key] = text
			}
		}
	case []any:
		for _, raw := range v {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			accountID := payloadString(item, "account_id")
			if accountID == "" {
				accountID = payloadString(item, "accountId")
			}
			if accountID == "" {
				accountID = payloadString(item, "id")
			}
			text := payloadString(item, "content")
			if text == "" {
				text = payloadString(item, "text")
			}
			if text == "" {
				text = payloadString(item, "override")
			}
			if accountID != "" && text != "" {
				overrides[accountID] = text
			}
		}
	case string:
		cleaned := strings.TrimSpace(v)
		lowered := strings.ToLower(cleaned)
		if cleaned == "" || lowered == "null" || lowered == "none" || lowered == "n/a" {
			return overrides
		}
		var parsed any
		if err := json.Unmarshal([]byte(cleaned), &parsed); err == nil {
			return coerceAccountContentOverride(parsed)
		}
	}
	return overrides
}

func normalizeTitle(value any, required bool) (string, error) {
	title := strings.TrimSpace(fmt.Sprintf("%v", value))
	if value == nil || title == "<nil>" {
		title = ""
	}
	runes := []rune(title)
	if len(runes) > 120 {
		title = strings.TrimRight(string(runes[:120]), " ")
	}
	if required && title == "" {
		return "", fmt.Errorf("LLM response missing title")
	}
	return title, nil
}

func parseVoiceResult(rawContent string, includeTitle, refineMode bool) (parsedVoiceResult, error) {
	payload, err := extractJSONObject(rawContent)
	if err != nil {
		return parsedVoiceResult{}, err
	}

	content := payloadString(payload, "content")
	if content == "" {
		return parsedVoiceResult{}, fmt.Errorf("LLM response missing content")
	}

	result := parsedVoiceResult{
		content:          content,
		hashtags:         payloadStringList(payload, "hashtags"),
		platformMetadata: payloadObject(payload, "platform_metadata"),
		overrides:        coerceAccountContentOverride(payload["account_content_override"]),
	}
	if includeTitle {
		title, _ := normalizeTitle(payload["title"], false)
		if title != "" {
			result.title = title
		} else if !refineMode {
			return parsedVoiceResult{}, fmt.Errorf("LLM response missing title")
		}
	}
	return result, nil
}
