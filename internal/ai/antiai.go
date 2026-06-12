package ai

import "strings"

// Lightweight defaults that steer voice quality without negative-programming
// overload. Teams can opt out via language_dna.anti_ai_override — then only
// their own banned words (if any) are used.

var qualityVoicePrinciples = []string{
	"Write like someone who actually lives this topic — conversational, sometimes blunt, never salesy.",
	"Prefer concrete facts and observations over adjectives. When unsure, say less instead of padding.",
	"Vary rhythm naturally: short punches, longer asides, fragments are fine. Do not sound polished or 'optimized'.",
}

// Only the worst universal tells — merged only when the team has fewer than
// defaultBannedWordLimit custom words.
var coreAvoidWords = []string{
	"tauche ein",
	"taucht ein",
	"tauchen ein",
	"game-changer",
	"revolutionär",
}

const defaultBannedWordLimit = 5

// cappedBannedWords returns at most limit banned words, prioritising
// team-specific terms.
func cappedBannedWords(profileBanned []string, override bool) []string {
	team := make([]string, 0, defaultBannedWordLimit)
	seen := map[string]bool{}
	for _, raw := range profileBanned {
		word := strings.TrimSpace(raw)
		if word == "" {
			continue
		}
		key := strings.ToLower(word)
		if seen[key] {
			continue
		}
		seen[key] = true
		team = append(team, word)
		if len(team) >= defaultBannedWordLimit {
			break
		}
	}

	if override {
		return team
	}

	for _, phrase := range coreAvoidWords {
		if len(team) >= defaultBannedWordLimit {
			break
		}
		key := strings.ToLower(strings.TrimSpace(phrase))
		if seen[key] {
			continue
		}
		seen[key] = true
		team = append(team, phrase)
	}
	return team
}
