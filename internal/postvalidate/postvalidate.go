// Package postvalidate holds transport-agnostic validation for post content
// against each destination account's provider capabilities (character limits and
// media requirements), honoring per-account content overrides.
//
// It mirrors the REST API's validatePostInput logic and is used by the MCP
// server, which previously skipped this validation entirely. Both rely on
// provider.Capabilities as the single source of truth for limits.
package postvalidate

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
)

// Destination is the per-account validation outcome.
type Destination struct {
	AccountID     string
	Provider      string
	Username      string
	MaxChars      int
	Length        int
	Valid         bool
	RequiresMedia bool
	MissingMedia  bool
}

// Result aggregates the per-destination validation.
type Result struct {
	Valid bool
	// MaxChars is the smallest non-zero character limit across destinations.
	MaxChars int
	// ContentLength is the rune length of the base content.
	ContentLength int
	Destinations  []Destination
}

// Problems returns a human-readable summary of every invalid destination,
// suitable for feeding back to an API caller or an AI agent so it can fix the
// content and retry. Returns "" when the result is valid.
func (r Result) Problems() string {
	var parts []string
	for _, d := range r.Destinations {
		if d.Valid {
			continue
		}
		who := d.AccountID
		if d.Username != "" {
			who = fmt.Sprintf("%s (id=%s)", d.Username, d.AccountID)
		}
		switch {
		case d.MissingMedia:
			parts = append(parts, fmt.Sprintf("%s on %s requires a media attachment", who, d.Provider))
		case d.MaxChars > 0 && d.Length > d.MaxChars:
			parts = append(parts, fmt.Sprintf("%s on %s allows %d characters but the text has %d", who, d.Provider, d.MaxChars, d.Length))
		default:
			parts = append(parts, fmt.Sprintf("%s on %s is invalid", who, d.Provider))
		}
	}
	return strings.Join(parts, "; ")
}

// Check validates the post content against each target account's provider
// capabilities, honoring per-account content overrides (input.EffectiveContent).
//
// accounts must be the already-loaded target accounts (callers are responsible
// for loading them and verifying team membership). Check does not special-case
// drafts; callers that allow oversized drafts should simply skip calling it.
func Check(ctx context.Context, providers *provider.Registry, accounts []domain.SocialAccount, input domain.CreatePostInput) (Result, error) {
	destinations := make([]Destination, 0, len(accounts))
	maxChars := 0
	allValid := true

	for _, account := range accounts {
		providerImpl, ok := providers.Get(account.Provider)
		if !ok {
			return Result{}, fmt.Errorf("account %s uses unsupported provider %q", account.ID, account.Provider)
		}
		capabilities, err := providerImpl.Capabilities(ctx, account)
		if err != nil {
			return Result{}, fmt.Errorf("capabilities for account %s: %w", account.ID, err)
		}

		effectiveContent := input.EffectiveContent(account.ID)
		contentLen := len([]rune(effectiveContent))
		isValid := capabilities.MaxChars == 0 || contentLen <= capabilities.MaxChars

		missingMedia := false
		if capabilities.RequiresMedia {
			effectiveMedia := domain.FilterMediaIDsForAccount(input.MediaIDs, input.MediaExcludeByAccount, account.ID)
			if len(effectiveMedia) == 0 {
				missingMedia = true
				isValid = false
			}
		}
		if !isValid {
			allValid = false
		}

		destinations = append(destinations, Destination{
			AccountID:     account.ID,
			Provider:      account.Provider,
			Username:      account.Username,
			MaxChars:      capabilities.MaxChars,
			Length:        contentLen,
			Valid:         isValid,
			RequiresMedia: capabilities.RequiresMedia,
			MissingMedia:  missingMedia,
		})
		if capabilities.MaxChars > 0 && (maxChars == 0 || capabilities.MaxChars < maxChars) {
			maxChars = capabilities.MaxChars
		}
	}

	slices.SortFunc(destinations, func(a, b Destination) int {
		return strings.Compare(a.AccountID, b.AccountID)
	})

	return Result{
		Valid:         allValid,
		MaxChars:      maxChars,
		ContentLength: len([]rune(input.Content)),
		Destinations:  destinations,
	}, nil
}
