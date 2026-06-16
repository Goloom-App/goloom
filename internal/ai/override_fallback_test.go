package ai

import (
	"testing"

	"git.f4mily.net/goloom/internal/domain"
)

func TestFillMissingOverridesCompressesPrimaryForLowerLimitAccount(t *testing.T) {
	mastodon := domain.AIAccountSummary{ID: "masto", Provider: "mastodon", MaxChars: 500}
	bluesky := domain.AIAccountSummary{ID: "bsky", Provider: "bluesky", MaxChars: 300}
	attempt := generateAttempt{
		primaryLimit:     mastodon.MaxChars,
		primaryAccountID: mastodon.ID,
		selectedAccounts: []domain.AIAccountSummary{mastodon, bluesky},
	}
	content := ""
	for len([]rune(content)) <= 470 {
		content += "word "
	}
	result := parsedVoiceResult{content: content}

	fillMissingOverrides(&result, attempt)

	override, ok := result.overrides[bluesky.ID]
	if !ok {
		t.Fatalf("expected a fallback override for the lower-limit account")
	}
	if got := len([]rune(override)); got > bluesky.MaxChars {
		t.Fatalf("fallback override %d chars exceeds limit %d", got, bluesky.MaxChars)
	}
	if err := validateLengths(result, attempt); err != nil {
		t.Fatalf("validateLengths should pass after fallback, got: %v", err)
	}
}

func TestFillMissingOverridesKeepsModelOverrideAndSkipsFittingAccounts(t *testing.T) {
	mastodon := domain.AIAccountSummary{ID: "masto", Provider: "mastodon", MaxChars: 500}
	bluesky := domain.AIAccountSummary{ID: "bsky", Provider: "bluesky", MaxChars: 300}
	attempt := generateAttempt{
		primaryLimit:     mastodon.MaxChars,
		primaryAccountID: mastodon.ID,
		selectedAccounts: []domain.AIAccountSummary{mastodon, bluesky},
	}
	// Primary content fits every account: no override should be invented.
	result := parsedVoiceResult{content: "short and snappy", overrides: map[string]string{}}
	fillMissingOverrides(&result, attempt)
	if _, ok := result.overrides[bluesky.ID]; ok {
		t.Fatalf("did not expect an override when the primary content already fits")
	}

	// A model-supplied override must be preserved untouched.
	result = parsedVoiceResult{
		content:   "this is a long primary text that clearly exceeds the bluesky limit when repeated enough to overflow three hundred characters of room so the override path triggers and the fallback would otherwise replace the model crafted variant which we do not want to happen here at all today",
		overrides: map[string]string{bluesky.ID: "model crafted punchy bluesky variant"},
	}
	fillMissingOverrides(&result, attempt)
	if result.overrides[bluesky.ID] != "model crafted punchy bluesky variant" {
		t.Fatalf("model override was overwritten: %q", result.overrides[bluesky.ID])
	}
}
