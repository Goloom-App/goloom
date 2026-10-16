package ai

import (
	"fmt"
	"strings"

	"git.f4mily.net/goloom/internal/domain"
)

// Social-media defaults live here and nowhere else. They apply only where the
// team's brand profile is silent, so a missing or partial profile never yields
// empty or contradictory instructions.

// platformDefault is the fallback hashtag behavior per platform.
type platformDefault struct {
	rule string // the full hashtag instruction rendered in the task prompt
}

var platformDefaults = map[string]platformDefault{
	"mastodon":  {"Use 1-3 readable CamelCase hashtags at the end of the post (also mirror them in the hashtags JSON field)."},
	"bluesky":   {"Use at most 2 concise hashtags, only when they add discovery value."},
	"friendica": {"Hashtags are allowed, but use only relevant ones; no filler tags."},
}

var fallbackPlatformDefault = platformDefault{"Use at most 2 hashtags, only when clearly relevant."}

// Brand-voice defaults, each used only when the matching profile field is empty.
const (
	defaultToneRule = "Use a natural, human tone — conversational, not marketing-speak."
	defaultHookRule = "Open with a hook; the first line must earn the read."
	defaultCTARule  = "Use at most one clear call to action."
	// defaultLanguageRule is used when the team has no preferred language.
	defaultLanguageRule = "match the language of the request and source material"
)

func platformDefaultFor(platform string) platformDefault {
	if d, ok := platformDefaults[strings.ToLower(strings.TrimSpace(platform))]; ok {
		return d
	}
	return fallbackPlatformDefault
}

// platformCharLimit derives the limit for a platform from the context's
// accounts (which already honor per-account overrides); with several accounts
// on the platform the smallest wins so the post fits all of them. Falls back to
// the provider default when no account is configured.
func platformCharLimit(context domain.AIContext, platform string) int {
	limit := 0
	for _, acc := range context.Accounts {
		if !strings.EqualFold(strings.TrimSpace(acc.Provider), strings.TrimSpace(platform)) || acc.MaxChars <= 0 {
			continue
		}
		if limit == 0 || acc.MaxChars < limit {
			limit = acc.MaxChars
		}
	}
	if limit == 0 {
		return domain.MaxCharsForProvider(platform, nil)
	}
	return limit
}

// brandDefaultRules returns the voice defaults for the parts of the profile
// that are empty. hasTone reports whether the profile already describes a voice.
func brandDefaultRules(style domain.StyleMetadata, hasTone bool) []string {
	var rules []string
	if !hasTone {
		rules = append(rules, defaultToneRule)
	}
	if style.ReachStrategy == nil || strings.TrimSpace(style.ReachStrategy.HookStyle) == "" {
		rules = append(rules, defaultHookRule)
	}
	if style.ReachStrategy == nil || strings.TrimSpace(style.ReachStrategy.CTAFocus) == "" {
		rules = append(rules, defaultCTARule)
	}
	return rules
}

// hashtagBudgetLine is the system-prompt hashtag line; it never contradicts
// the task prompt's platform rule.
func hashtagBudgetLine(maxHashtags int) string {
	if maxHashtags > 0 {
		return fmt.Sprintf("up to %d", maxHashtags)
	}
	return "platform defaults (see task)"
}

// hasVoiceDescription reports whether the profile says anything about the
// brand's tone (as opposed to only hook/CTA strategy or identity facts).
func hasVoiceDescription(style domain.StyleMetadata) bool {
	if strings.TrimSpace(style.Tonality) != "" {
		return true
	}
	if id := style.Identity; id != nil && (strings.TrimSpace(id.Persona) != "" || strings.TrimSpace(id.Archetype) != "") {
		return true
	}
	if dna := style.LanguageDNA; dna != nil && (strings.TrimSpace(dna.SentenceStyle) != "" || strings.TrimSpace(dna.HumorStyle) != "") {
		return true
	}
	return false
}
