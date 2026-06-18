package ai

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"git.f4mily.net/goloom/internal/domain"
)

// Platform constraints mirrored from the team validation rules.
var platformLimits = map[string]int{
	"mastodon":  500,
	"bluesky":   300,
	"friendica": 5000,
}

var platformHashtagRules = map[string]string{
	"mastodon":  "Use readable hashtags sparingly, ideally 1-3 at the end.",
	"bluesky":   "Use at most 2 concise hashtags only when they add discovery value.",
	"friendica": "Hashtags are allowed, but keep them relevant and non-spammy.",
	"default":   "Use hashtags sparingly and only when they are clearly relevant.",
}

var outputFormatHints = map[string]string{
	"post":   "Single post — structure is up to you.",
	"teaser": "Short teaser that builds curiosity; hook first, link or CTA second.",
	"poll":   "Poll question with 2-4 answer options woven into the text.",
	"thread": "Opening post of a thread; do not summarise the whole thread.",
}

var moodAdjustmentHints = map[string]string{
	"more_expertise":         "Lean on concrete facts from the source material; show domain depth.",
	"shorter_punchier":       "Cut length aggressively. Every word must earn its place.",
	"remove_marketing_speak": "Strip hype adjectives; replace with specifics.",
}

const (
	brandStyleExampleLimit   = 3
	taskRecentPostLimit      = 3
	recurringRecentPostLimit = 6
	formattingRuleLimit      = 4
	recentPostExcerptChars   = 140
)

const recurringUserRequestBase = "Write a fresh social post from a recurring template.\n" +
	"Grounding — every specific claim (dates, times, places, names, prices, offers, " +
	"links, numbers, agenda items) must come from one of:\n" +
	"- the expanded template below\n" +
	"- the editorial direction below (if provided)\n" +
	"- the paired announcement reference (if provided)\n" +
	"Enhancement — where AI adds value:\n" +
	"- sharper wording, rhythm, and CTA; more engaging but faithful copy\n" +
	"- a new opening and structure vs. the template and recent posts\n" +
	"- persuasive emphasis on what the sources already state (event, campaign, meetup, product, etc.)\n" +
	"Do not add factual specifics from brand knowledge, industry profile, or recent posts — " +
	"use those only for tone and deduplication."

type platformConstraints struct {
	platform    string
	charLimit   int
	hashtagRule string
}

// BuildSystemPrompt renders the team's brand-voice system prompt.
func BuildSystemPrompt(context domain.AIContext) string {
	return buildBrandVoicePrompt(context)
}

// BuildGenerationPromptFromParams renders the per-request task prompt from raw
// JSON job params (used by the prompt preview endpoint).
func BuildGenerationPromptFromParams(context domain.AIContext, rawParams json.RawMessage, platform string) (string, error) {
	p := params{}
	if len(rawParams) > 0 {
		if err := json.Unmarshal(rawParams, &p); err != nil {
			return "", fmt.Errorf("parse params: %w", err)
		}
	}
	return buildGenerationPrompt(context, p, platform), nil
}

func styleMetadata(context domain.AIContext) domain.StyleMetadata {
	if context.Profile == nil {
		return domain.StyleMetadata{}
	}
	return context.Profile.StyleMetadata
}

func buildBrandVoicePrompt(context domain.AIContext) string {
	style := styleMetadata(context)
	teamName := strings.TrimSpace(context.Team.Name)
	if teamName == "" {
		teamName = "unknown team"
	}

	antiAIOverride := style.LanguageDNA != nil && style.LanguageDNA.AntiAIOverride
	bannedWords := cappedBannedWords(style.BannedWords, antiAIOverride)
	var preferredWords, signaturePhrases []string
	if style.LanguageDNA != nil {
		preferredWords = nonEmptyStrings(style.LanguageDNA.PreferredWords)
		signaturePhrases = nonEmptyStrings(style.LanguageDNA.SignaturePhrases)
	}
	formattingRules := nonEmptyStrings(style.FormattingRules)
	if len(formattingRules) > formattingRuleLimit {
		formattingRules = formattingRules[:formattingRuleLimit]
	}

	var knowledgeSources []string
	for _, item := range context.KnowledgeSources {
		knowledgeSources = append(knowledgeSources, formatKnowledgeSource(item))
	}

	var styleExamples []string
	for _, item := range context.StyleExamples {
		content := strings.TrimSpace(item.Content)
		if content != "" {
			styleExamples = append(styleExamples, content)
		}
		if len(styleExamples) >= brandStyleExampleLimit {
			break
		}
	}

	preferredLanguage := strings.TrimSpace(style.PreferredLanguage)
	if preferredLanguage == "" {
		preferredLanguage = "unspecified"
	}

	voiceSummary := brandVoiceSummary(style)
	if voiceSummary == "" {
		voiceSummary = "Write clearly and authentically for this account."
	}

	sections := []string{
		fmt.Sprintf("You write social media posts for %q.", teamName),
		"",
		"Brand voice:",
		voiceSummary,
		"",
		"Quality bar:",
	}
	if antiAIOverride {
		sections = append(sections, formatList(nil))
	} else {
		sections = append(sections, formatList(qualityVoicePrinciples))
	}

	if len(formattingRules) > 0 {
		sections = append(sections,
			"",
			"Style notes (soft guidelines — not a checklist; examples show patterns, not phrases to copy verbatim):",
			formatList(formattingRules),
		)
	}
	if len(preferredWords) > 0 {
		sections = append(sections,
			"",
			"Words that fit this account (only when relevant to this specific post — never force):",
			formatInlineList(preferredWords),
		)
	}
	if len(signaturePhrases) > 0 {
		sections = append(sections,
			"",
			"Signature phrases (only when they fit perfectly):",
			formatInlineList(signaturePhrases),
		)
	}
	if len(bannedWords) > 0 {
		sections = append(sections,
			"",
			"Especially avoid these words/phrases:",
			formatInlineList(bannedWords),
		)
	}
	if len(knowledgeSources) > 0 {
		sections = append(sections,
			"",
			"Brand knowledge base (static facts about us — not about the specific item you are posting):",
			formatList(knowledgeSources),
		)
	}

	sections = append(sections,
		"",
		"Posts that sound like us (match tone and attitude, not structure or layout):",
		formatStyleExamples(styleExamples),
		"",
		fmt.Sprintf("Language: %s | Hashtag budget: up to %d", preferredLanguage, style.MaxHashtags),
		"Facts about the specific item you are posting about always come from the source material in the task message.",
		untrustedSourceGuard,
	)

	return strings.TrimSpace(strings.Join(sections, "\n"))
}

func brandVoiceSummary(style domain.StyleMetadata) string {
	var paragraphs []string

	if style.Identity != nil {
		persona := strings.TrimSpace(style.Identity.Persona)
		archetype := strings.TrimSpace(style.Identity.Archetype)
		if persona != "" {
			paragraphs = append(paragraphs, persona)
		} else if archetype != "" {
			paragraphs = append(paragraphs, fmt.Sprintf("This is a %s account.", archetype))
		}

		var contextBits []string
		for _, bit := range []string{style.Identity.Industry, style.Identity.MainValue, style.Identity.TargetAudience} {
			if strings.TrimSpace(bit) != "" {
				contextBits = append(contextBits, strings.TrimSpace(bit))
			}
		}
		if len(contextBits) > 0 {
			paragraphs = append(paragraphs, strings.Join(contextBits, " "))
		}
	}

	var voiceBits []string
	if style.LanguageDNA != nil {
		if s := strings.TrimSpace(style.LanguageDNA.SentenceStyle); s != "" {
			voiceBits = append(voiceBits, s)
		}
		if s := strings.TrimSpace(style.LanguageDNA.HumorStyle); s != "" {
			voiceBits = append(voiceBits, "Humor: "+s)
		}
	}
	if style.ReachStrategy != nil {
		if s := strings.TrimSpace(style.ReachStrategy.HookStyle); s != "" {
			voiceBits = append(voiceBits, "Hooks: "+s)
		}
		if s := strings.TrimSpace(style.ReachStrategy.CTAFocus); s != "" {
			voiceBits = append(voiceBits, "CTAs: "+s)
		}
	}
	if len(voiceBits) > 0 {
		paragraphs = append(paragraphs, strings.Join(voiceBits, " "))
	}

	return strings.Join(paragraphs, "\n\n")
}

func formatKnowledgeSource(item domain.KnowledgeSource) string {
	name := strings.TrimSpace(item.Name)
	if name == "" {
		name = "source"
	}
	content := strings.TrimSpace(item.Content)
	if len(content) > 4000 {
		content = content[:4000] + "…"
	}
	prefix := "[" + name + "]"
	if url := strings.TrimSpace(item.SourceURL); url != "" {
		prefix += " (" + url + ")"
	}
	if content == "" {
		content = "No extracted content"
		return prefix + "\n" + content
	}
	return prefix + "\n" + wrapUntrusted(content)
}

// buildGenerationPrompt renders the per-request task prompt.
func buildGenerationPrompt(context domain.AIContext, p params, platform string) string {
	style := styleMetadata(context)
	constraints := applyPlatformConstraints(platform, style.MaxHashtags)
	recentLimit := taskRecentPostLimit
	if isRecurringKind(recurringPostKind(p)) {
		recentLimit = recurringRecentPostLimit
	}

	return renderTaskPrompt(taskPromptInput{
		platform:          constraints.platform,
		charLimit:         constraints.charLimit,
		hashtagRule:       constraints.hashtagRule,
		userRequest:       resolveUserRequest(p),
		sourceMaterial:    sourceMaterial(p, context),
		recentPosts:       recentPostExcerpts(context, recentLimit),
		campaignHint:      campaignTaskHint(context, p),
		outputFormat:      outputFormatHint(p),
		moodAdjustments:   moodAdjustments(p),
		technicalNotes:    technicalNotes(p),
		outputConstraints: outputConstraints(context, p),
		recurringPlan:     recurringPublicationPlan(p, context),
	})
}

type taskPromptInput struct {
	platform          string
	charLimit         int
	hashtagRule       string
	userRequest       string
	sourceMaterial    []string
	recentPosts       []string
	campaignHint      string
	outputFormat      string
	moodAdjustments   []string
	technicalNotes    []string
	outputConstraints []string
	recurringPlan     string
}

func renderTaskPrompt(in taskPromptInput) string {
	userRequest := strings.TrimSpace(in.userRequest)
	if userRequest == "" {
		userRequest = "Write a platform-ready post for this account."
	}
	sections := []string{"## Task", userRequest}

	if strings.TrimSpace(in.recurringPlan) != "" {
		sections = append(sections, "", "## Publication plan", strings.TrimSpace(in.recurringPlan))
	}
	if len(in.sourceMaterial) > 0 {
		sections = append(sections, "", "## Source material")
		sections = append(sections, in.sourceMaterial...)
	}
	if len(in.recentPosts) > 0 {
		sections = append(sections,
			"",
			"## Do not repeat",
			"Recent posts below are for deduplication only — do not copy their openings, structure, phrasing, or subject matter.",
			formatList(in.recentPosts),
		)
	}
	if strings.TrimSpace(in.campaignHint) != "" {
		sections = append(sections, "", "## Campaign goal", strings.TrimSpace(in.campaignHint))
	}

	formatHint := ""
	if in.outputFormat != "" {
		formatHint = "\n- Output shape: " + in.outputFormat
	}
	moodHint := ""
	if len(in.moodAdjustments) > 0 {
		moodHint = "\n\nMood for this draft:\n" + formatList(in.moodAdjustments)
	}

	sections = append(sections,
		"",
		"## Platform",
		"- Platform: "+in.platform,
		fmt.Sprintf("- Character limit: %d", in.charLimit),
		"- Hashtag guidance: "+in.hashtagRule+formatHint,
	)

	if len(in.technicalNotes) > 0 {
		sections = append(sections, "", "## Technical notes", formatList(in.technicalNotes))
	}
	if len(in.outputConstraints) > 0 {
		sections = append(sections, "", "## Output constraints", formatList(in.outputConstraints))
	}

	sections = append(sections,
		moodHint,
		"",
		"Respond with a JSON object using this exact structure (no markdown, no code fences):",
		`{"content": "the post text including hashtags at the end", "hashtags": ["hashtag1", "hashtag2"], "platform_metadata": {"key": "value"}}`,
		"Hashtags must appear in content, not only in the hashtags array.",
	)

	return strings.TrimSpace(strings.Join(sections, "\n"))
}

func applyPlatformConstraints(platform string, maxHashtags int) platformConstraints {
	normalized := strings.ToLower(strings.TrimSpace(platform))
	key := normalized
	if key == "" {
		key = "default"
	}
	charLimit, ok := platformLimits[normalized]
	if !ok {
		charLimit = 500
	}
	return platformConstraints{
		platform:    key,
		charLimit:   charLimit,
		hashtagRule: hashtagRule(normalized, maxHashtags),
	}
}

func hashtagRule(platform string, maxHashtags int) string {
	if maxHashtags > 0 {
		minimum := maxHashtags
		if maxHashtags >= 3 {
			minimum = 3
		}
		return fmt.Sprintf("Include %d to %d relevant hashtags at the end of the post text (also mirror them in the hashtags JSON field).", minimum, maxHashtags)
	}
	if rule, ok := platformHashtagRules[platform]; ok {
		return rule
	}
	return platformHashtagRules["default"]
}

func outputConstraints(context domain.AIContext, p params) []string {
	var constraints []string
	style := styleMetadata(context)
	german := strings.HasPrefix(strings.ToLower(strings.TrimSpace(orDefault(style.PreferredLanguage, "en"))), "de")
	recurring := p != nil && isRecurringKind(recurringPostKind(p))

	if recurring {
		constraints = append(constraints, recurringOutputConstraints(german)...)
	}
	for _, rule := range nonEmptyStrings(style.FormattingRules) {
		if strings.Contains(strings.ToLower(rule), "emoji") {
			constraints = append(constraints, "Respect this emoji rule: "+rule)
		}
	}

	if style.MaxHashtags > 0 {
		if recurring {
			constraints = append(constraints, recurringHashtagConstraint(style.MaxHashtags, german))
		} else {
			constraints = append(constraints, fmt.Sprintf("Hashtags are important for reach — use up to %d relevant tags derived from the source topics.", style.MaxHashtags))
		}
	}

	constraints = append(constraints,
		"Style-note examples (e.g. sample sentence patterns) are not catchphrases — do not paste them unless they genuinely fit this item.",
	)
	return constraints
}

func recurringOutputConstraints(german bool) []string {
	if german {
		return []string{
			"Neuer Text — keine Sätze oder Einstiege aus der Vorlage oder den letzten Posts recyceln.",
			"Keine erfundenen Fakten: Orte, Preise, Rabatte, Agenda-Themen, Produkte oder Gäste, " +
				"die nicht in Vorlage, redaktioneller Anweisung oder Ankündigungs-Referenz stehen.",
			"Redaktionelle Anweisung darf Betonung und Winkel steuern — aber keine neuen Fakten einführen.",
			"Brand-Wissen und letzte Posts nur für Ton und Deduplizierung — nicht als Themenquelle.",
		}
	}
	return []string{
		"Fresh wording — do not recycle template sentences or recent post openings.",
		"No invented facts: venues, prices, discounts, agenda topics, products, or guests " +
			"unless stated in the template, editorial direction, or announcement reference.",
		"Editorial direction may shape emphasis and angle — but cannot introduce new facts.",
		"Brand knowledge and recent posts are for tone and dedup only — not topic sources.",
	}
}

func recurringHashtagConstraint(maxHashtags int, german bool) string {
	if german {
		return fmt.Sprintf(
			"Bis zu %d Hashtags aus Vorlage oder redaktioneller Anweisung — "+
				"keine generischen Branchen-/Hobby-Tags aus dem Brand-Profil, "+
				"wenn sie nicht wörtlich in diesen Quellen stehen.", maxHashtags)
	}
	return fmt.Sprintf(
		"Use up to %d hashtags grounded in the template or editorial direction — "+
			"not generic industry/hobby tags from the brand profile unless literal in those sources.", maxHashtags)
}

func recentPostExcerpts(context domain.AIContext, limit int) []string {
	var excerpts []string
	for _, post := range context.RecentPosts {
		text := strings.TrimSpace(post.Content)
		if text == "" {
			text = strings.TrimSpace(post.Title)
		}
		if text == "" {
			continue
		}
		compact := strings.Join(strings.Fields(text), " ")
		runes := []rune(compact)
		if len(runes) > recentPostExcerptChars {
			compact = strings.TrimRight(string(runes[:recentPostExcerptChars]), " ") + "…"
		}
		excerpts = append(excerpts, compact)
		if len(excerpts) >= limit {
			break
		}
	}
	return excerpts
}

func hasRSSSource(p params) bool {
	return p.str("rss_article_title") != "" ||
		p.str("rss_article_content", "rss_article_summary") != "" ||
		p.str("rss_article_link") != ""
}

func recurringPostKind(p params) string {
	if kind := strings.ToLower(p.str("recurring_post_kind")); kind != "" {
		return kind
	}
	if auto := p.nested("recurring_automation"); auto != nil {
		return strings.ToLower(auto.str("post_kind"))
	}
	return ""
}

func isRecurringKind(kind string) bool {
	return kind == "announcement" || kind == "main"
}

func paramScheduleValue(p params, key, nestedKey, nestedField string) string {
	if direct := p.str(key); direct != "" {
		return direct
	}
	if auto := p.nested(nestedKey); auto != nil {
		return auto.str(nestedField)
	}
	return ""
}

func recurringPublicationPlan(p params, context domain.AIContext) string {
	kind := recurringPostKind(p)
	if !isRecurringKind(kind) {
		return ""
	}

	language := orDefault(styleMetadata(context).PreferredLanguage, "en")
	german := strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "de")
	mainAt := paramScheduleValue(p, "main_event_at", "recurring_automation", "template_occurrence_at")
	mainLabel := ""
	if mainAt != "" {
		mainLabel = formatScheduleLabel(mainAt, language)
	}

	var lines []string
	if kind == "announcement" {
		if german {
			lines = append(lines,
				"Rolle: ANKÜNDIGUNG (wird vor dem Event veröffentlicht).",
				"Die Vorlage liefert Fakten und Zeitform (z. B. „Am Freitag …“ statt „heute“) — frisch und einladend formulieren, ohne neue Fakten.",
			)
		} else {
			lines = append(lines,
				"Role: ANNOUNCEMENT (published before the event).",
				"The template supplies facts and timing (e.g. a weekday/date vs. “today”) — write fresh, inviting copy without adding new facts.",
			)
		}
	} else {
		if german {
			lines = append(lines,
				"Rolle: HAUPTPOST (wird am Event-Tag veröffentlicht).",
				"Die Vorlage liefert Fakten und Zeitform (z. B. „heute Abend“) — frisch und mitreißend formulieren, ohne neue Fakten.",
			)
		} else {
			lines = append(lines,
				"Role: MAIN EVENT (published on the event day).",
				"The template supplies facts and timing (e.g. “tonight”) — write fresh, energetic copy without adding new facts.",
			)
		}
	}

	// The publication/posting time is a scheduling-only field: the system sets it
	// automatically. It is NOT event information — the model used to mistake the
	// posting time for the event time and write it into the post. Never surface
	// it; event timing comes only from the template above.
	if german {
		lines = append(lines, "Die Veröffentlichungszeit dieses Posts wird automatisch gesetzt — niemals im Text erwähnen. Zeit- und Datumsangaben ausschließlich aus der Vorlage übernehmen.")
	} else {
		lines = append(lines, "This post's publication time is set automatically — never mention it in the text. Take any time or date wording only from the template above.")
	}

	if mainLabel != "" && kind == "announcement" {
		if german {
			lines = append(lines, "Event-Datum: "+mainLabel)
		} else {
			lines = append(lines, "Event date: "+mainLabel)
		}
	}
	return strings.Join(lines, "\n")
}

func recurringTemplateSource(sourceContent string, context domain.AIContext) string {
	german := strings.HasPrefix(strings.ToLower(strings.TrimSpace(orDefault(styleMetadata(context).PreferredLanguage, "en"))), "de")
	if german {
		return "Wiederkehrende Vorlage (ausgefüllt — Fakten übernehmen, Wortlaut neu schreiben):\n---\n" + sourceContent + "\n---\n" +
			"Alle Fakten oben beibehalten (Zeit, Ort, Angebot, Links, Zahlen, Namen).\n" +
			"Neu formulieren und betonen — keine zusätzlichen Fakten jenseits der Quellen in der Aufgabe."
	}
	return "Recurring template (expanded — keep facts, rewrite wording):\n---\n" + sourceContent + "\n---\n" +
		"Carry over every fact above (timing, venue, offer, links, numbers, names).\n" +
		"Rephrase and emphasize — no extra facts beyond the grounding sources in the task."
}

func hasWebPageSource(p params) bool {
	return p.str("source_url", "sourceUrl") != ""
}

func webPageSourceSection(p params) string {
	sourceURL := p.str("source_url", "sourceUrl")
	sourceContent := p.str("source_content", "existing_content")
	pageTitle := p.str("page_title")
	lines := []string{"PAGE SOURCE (primary factual basis — every specific claim must come from here):"}
	if sourceURL != "" {
		lines = append(lines, "Link: "+sourceURL)
	}
	if pageTitle != "" {
		lines = append(lines, "Title: "+pageTitle)
	}
	if sourceContent != "" {
		lines = append(lines, "Content:\n"+wrapUntrusted(sourceContent))
	} else if sourceURL != "" {
		lines = append(lines, "Page content could not be extracted — keep claims minimal and do not invent article body text.")
	}
	lines = append(lines, "Use the link above in the post. Base claims only on the title/content above — do not invent details.")
	return strings.Join(lines, "\n")
}

func sourceMaterial(p params, context domain.AIContext) []string {
	var sections []string

	if hasWebPageSource(p) {
		sections = append(sections, webPageSourceSection(p))
	}

	rssTitle := p.str("rss_article_title")
	rssLink := p.str("rss_article_link")
	rssContent := p.str("rss_article_content", "rss_article_summary")
	if rssTitle != "" || rssLink != "" || rssContent != "" {
		lines := []string{"RSS ITEM (primary factual source — every specific claim must come from here):"}
		if rssTitle != "" {
			lines = append(lines, "Title: "+rssTitle)
		}
		if rssLink != "" {
			lines = append(lines, "Link: "+rssLink)
		}
		if rssContent != "" {
			lines = append(lines, "Text:\n"+wrapUntrusted(rssContent))
		}
		lines = append(lines,
			"The title and link above are authoritative. "+
				"Pick 2-4 concrete points from the body text and work them in naturally. "+
				"Do not invent facts, change numbers/identifiers, or swap the item link for a feed URL.")
		sections = append(sections, strings.Join(lines, "\n"))
	}

	skeleton := p.str("post_skeleton")
	sourceContent := p.str("source_content", "existing_content")
	switch {
	case skeleton != "" && hasRSSSource(p):
		sections = append(sections, "RSS post skeleton (optional layout/CTA hints only — not factual content):\n"+wrapUntrusted(skeleton))
	case sourceContent != "" && hasRSSSource(p):
		sections = append(sections, "Additional provided source text (supplements the RSS item; keep RSS title/link authoritative):\n"+wrapUntrusted(sourceContent))
	case sourceContent != "" && !hasWebPageSource(p):
		if isRecurringKind(recurringPostKind(p)) {
			sections = append(sections, recurringTemplateSource(sourceContent, context))
		} else {
			sections = append(sections, "Previous draft (facts and tone reference only — do not copy structure or layout verbatim):\n"+wrapUntrusted(sourceContent))
		}
	}

	announcement := p.str("announcement_reference_content")
	announcementTitle := p.str("announcement_reference_title")
	if announcement != "" || announcementTitle != "" {
		lines := []string{"Paired announcement to stay consistent with:"}
		if announcementTitle != "" {
			lines = append(lines, "Title: "+announcementTitle)
		}
		if announcement != "" {
			lines = append(lines, "Text:\n"+wrapUntrusted(announcement))
		}
		sections = append(sections, strings.Join(lines, "\n"))
	}

	return sections
}

func campaignTaskHint(context domain.AIContext, p params) string {
	campaignFormatID := p.str("campaign_format_id", "campaignFormatId")
	if campaignFormatID == "" {
		return ""
	}
	for _, item := range context.CampaignFormats {
		if item.ID != campaignFormatID {
			continue
		}
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = "Campaign"
		}
		lines := []string{
			"Campaign: " + name,
			"Treat any template as a goal, not a form to fill in — vary openings and structure.",
		}
		if structure := formatRawJSON(item.Structure); structure != "" {
			lines = append(lines, "Suggested elements (reorder or adapt freely): "+structure)
		}
		if hashtags := nonEmptyStrings(item.RequiredHashtags); len(hashtags) > 0 {
			lines = append(lines, "Required hashtags: "+strings.Join(hashtags, ", "))
		}
		return strings.Join(lines, "\n")
	}
	return ""
}

func outputFormatHint(p params) string {
	raw := strings.ToLower(p.str("output_format", "format"))
	return outputFormatHints[raw]
}

func moodAdjustments(p params) []string {
	var hints []string
	appendHint := func(key string) {
		hint, ok := moodAdjustmentHints[key]
		if !ok {
			return
		}
		for _, existing := range hints {
			if existing == hint {
				return
			}
		}
		hints = append(hints, hint)
	}

	if flags, ok := p["mood_adjustments"].([]any); ok {
		for _, flag := range flags {
			appendHint(strings.TrimSpace(fmt.Sprintf("%v", flag)))
		}
	} else if flags, ok := p["moodAdjustments"].([]any); ok {
		for _, flag := range flags {
			appendHint(strings.TrimSpace(fmt.Sprintf("%v", flag)))
		}
	}
	for _, key := range []string{"more_expertise", "shorter_punchier", "remove_marketing_speak"} {
		if value, ok := p[key].(bool); ok && value {
			appendHint(key)
		}
	}
	return hints
}

func resolveUserRequest(p params) string {
	editorial := p.str("occasion", "prompt_hint", "content_hint", "request", "prompt", "instruction")

	if hasWebPageSource(p) {
		base := "Write a social post based on the provided page link and content below.\n" +
			"- Use the link from the source material in the post.\n" +
			"- Every specific claim must come from the provided content text.\n" +
			"- Do not invent facts, quotes, or details not supported by the source."
		if editorial != "" {
			return base + "\n\nEditorial direction: " + editorial
		}
		return base
	}

	if hasRSSSource(p) {
		base := "Write a new social post based on the RSS feed item below.\n" +
			"- Use the exact title from the source (keep any numbers or identifiers).\n" +
			"- Use the item link from the source — never the RSS subscription/feed URL.\n" +
			"- Mention 2-3 concrete details from the body text; do not invent themes or change identifiers.\n" +
			"- Do not force brand buzzwords unless they appear in the source."
		if editorial != "" {
			return base + "\n\nEditorial direction: " + editorial
		}
		return base
	}

	if isRecurringKind(recurringPostKind(p)) {
		if editorial != "" {
			return recurringUserRequestBase + "\n\n" +
				"Editorial direction (shapes emphasis and angle — cannot add facts beyond the template):\n" + editorial
		}
		return recurringUserRequestBase
	}

	if editorial != "" {
		return editorial
	}
	return "Write a post that fits this account and the source material."
}

var technicalNoteSkipKeys = map[string]bool{
	"prompt_hint": true, "content_hint": true, "request": true, "prompt": true,
	"instruction": true, "occasion": true, "mood_adjustments": true, "moodAdjustments": true,
	"more_expertise": true, "shorter_punchier": true, "remove_marketing_speak": true,
	"source_content": true, "existing_content": true, "source_url": true, "sourceUrl": true,
	"page_title": true, "post_skeleton": true, "rss_feed_url": true, "refine_content": true,
	"refine": true, "rss_article_title": true, "rss_article_content": true,
	"rss_article_summary": true, "rss_article_link": true,
	"announcement_reference_content": true, "announcement_reference_title": true,
	"campaign_format_id": true, "campaignFormatId": true, "output_format": true,
	"format": true, "platform": true, "recurring_post_kind": true, "recurring_automation": true,
	"post_scheduled_at": true, "main_event_at": true, "days_before_main_event": true,
	"template_occurrence_at": true,
}

func technicalNotes(p params) []string {
	keys := make([]string, 0, len(p))
	for key := range p {
		if technicalNoteSkipKeys[key] || strings.HasSuffix(key, "_at") || strings.HasSuffix(key, "At") {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	notes := make([]string, 0, len(keys))
	for _, key := range keys {
		notes = append(notes, key+": "+formatValue(p[key]))
	}
	return notes
}

// buildVibePreviewPrompt summarizes the brand voice in one or two sentences.
func buildVibePreviewPrompt(context domain.AIContext) string {
	teamName := strings.TrimSpace(context.Team.Name)
	if teamName == "" {
		teamName = "unknown team"
	}
	return "You summarize a team's social media brand voice in one or two sentences, in German if the profile language is de, otherwise English.\n\n" +
		"Team: " + teamName + "\n" +
		"Profile:\n" + brandVoiceSummary(styleMetadata(context)) + "\n\n" +
		"Respond with ONLY valid JSON (no markdown):\n" +
		`{"summary": "Ich klinge jetzt wie ...", "suggestion": "Optional one-line tweak suggestion or empty string"}`
}

// buildProfileAssistantPrompt generates a brand profile proposal from a brief.
func buildProfileAssistantPrompt(p params) (string, error) {
	brief := p.str("brief", "description")
	if brief == "" {
		return "", fmt.Errorf("profile_assistant requires a non-empty brief")
	}
	examples := p.stringList("examples", "reference_posts")
	language := orDefault(p.str("language"), "de")

	examplesBlock := ""
	if len(examples) > 0 {
		examplesBlock = "\nExisting reference posts or quotes (mirror their voice):\n" + formatList(examples) + "\n"
	}

	return `You design social-media brand profiles for the Goloom scheduler.
Your output is consumed directly by a prompt builder, so be specific and concrete.

A user described their account or project. Propose a complete profile that
sounds genuinely human — never like generic AI marketing copy.

Profile language preference: ` + language + `
User brief:
"""
` + brief + `
"""
` + examplesBlock + `Rules:
- Match the brief's domain precisely (a dentist sounds nothing like a tech podcast).
- Persona must read like a real person, not a corporate role.
- archetype is a 2-5 word label (e.g. "Tech Podcast", "Solo Indie Dev", "Zahnarztpraxis", "Boutique Werbeagentur").
- preferred_words and signature_phrases must be domain-specific, not generic.
- banned_words: at most 5 words this account should especially avoid.
- formatting_rules: 2-4 soft style notes, not rigid laws.
- main_value: one concrete sentence; no buzzwords.

Respond with ONLY valid JSON (no markdown, no code fences) matching this exact schema:
{
  "identity": {
    "archetype": "...",
    "persona": "...",
    "industry": "...",
    "main_value": "...",
    "target_audience": "..."
  },
  "language_dna": {
    "sentence_style": "...",
    "humor_style": "...",
    "preferred_words": ["..."],
    "signature_phrases": ["..."]
  },
  "reach_strategy": {
    "hook_style": "...",
    "cta_focus": "..."
  },
  "banned_words": ["..."],
  "formatting_rules": ["..."],
  "preferred_language": "` + language + `",
  "max_hashtags": 3
}`, nil
}

func formatList(items []string) string {
	if len(items) == 0 {
		return "- None provided."
	}
	lines := make([]string, 0, len(items))
	for _, item := range items {
		lines = append(lines, "- "+item)
	}
	return strings.Join(lines, "\n")
}

func formatInlineList(items []string) string {
	if len(items) == 0 {
		return "- None."
	}
	return "- " + strings.Join(items, ", ")
}

func formatStyleExamples(items []string) string {
	if len(items) == 0 {
		return "- None provided — write in the brand voice described above."
	}
	blocks := make([]string, 0, len(items))
	for index, item := range items {
		blocks = append(blocks, fmt.Sprintf("Example %d:\n---\n%s\n---", index+1, strings.TrimSpace(item)))
	}
	return strings.Join(blocks, "\n\n")
}

func formatRawJSON(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" || trimmed == "{}" || trimmed == "[]" {
		return ""
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return trimmed
	}
	encoded, err := marshalSorted(decoded)
	if err != nil {
		return trimmed
	}
	return encoded
}

func nonEmptyStrings(values []string) []string {
	var out []string
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}

func orDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
