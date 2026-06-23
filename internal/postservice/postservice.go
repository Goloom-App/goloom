// Package postservice is the single pipeline every interactive post create/update
// path (REST and MCP) runs through. It normalizes the input, enforces the domain
// shape invariants, verifies the target accounts (they exist, belong to the
// team, and every per-account override references a target so none is silently
// dropped), and—when requested—validates per-account character/media limits.
//
// Centralizing this means a new caller cannot accidentally skip a validation
// step: the only correct way to prepare a post is Prepare.
package postservice

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/postvalidate"
	"git.f4mily.net/goloom/internal/provider"
)

// AccountLoader is the slice of the store the pipeline needs.
type AccountLoader interface {
	GetAccountsByIDsGlobal(ctx context.Context, ids []string) ([]domain.SocialAccount, error)
}

// Service prepares and validates posts.
type Service struct {
	accounts  AccountLoader
	providers *provider.Registry
}

// New builds a Service.
func New(accounts AccountLoader, providers *provider.Registry) *Service {
	return &Service{accounts: accounts, providers: providers}
}

// Options tunes the pipeline per caller.
type Options struct {
	// CheckLimits enforces per-account character/media limits. Callers set it
	// false for drafts, which may be oversized until they are scheduled.
	CheckLimits bool
}

// Result is the outcome of Prepare.
type Result struct {
	// Input is the normalized post, ready to persist.
	Input domain.CreatePostInput
	// EffectiveTeam is the team the target accounts belong to (or the requested
	// team when there are no targets, e.g. an empty draft).
	EffectiveTeam string
	// Validation holds the per-destination character/media report. It is only
	// populated when Options.CheckLimits is set and there are target accounts;
	// otherwise it is the zero value with Valid=true.
	Validation postvalidate.Result
}

// Prepare normalizes and validates a post. teamID is the expected team; when
// non-empty every target account must belong to it. Hard failures (shape
// invariants, unknown/cross-team account, misdirected override, unsupported
// provider) are returned as an error. Character/media validity is reported in
// Result.Validation so preview callers can surface it without failing; mutating
// callers should treat !Result.Validation.Valid as a rejection.
func (s *Service) Prepare(ctx context.Context, teamID string, input domain.CreatePostInput, opts Options) (Result, error) {
	// Capture the override keys the caller supplied before normalization drops
	// any that do not match a target — so a misdirected override is reported
	// instead of silently lost.
	rawOverrideKeys := make([]string, 0, len(input.AccountContentOverride))
	for id, content := range input.AccountContentOverride {
		if strings.TrimSpace(content) != "" {
			rawOverrideKeys = append(rawOverrideKeys, id)
		}
	}

	input.Normalize()
	if err := input.Validate(); err != nil {
		return Result{}, err
	}

	accounts, effectiveTeam, err := s.ResolveTargets(ctx, teamID, input.TargetAccounts, rawOverrideKeys)
	if err != nil {
		return Result{}, err
	}

	res := Result{Input: input, EffectiveTeam: effectiveTeam, Validation: postvalidate.Result{Valid: true}}
	if opts.CheckLimits && len(accounts) > 0 {
		validation, err := s.providersCheck(ctx, accounts, input)
		if err != nil {
			return Result{}, err
		}
		res.Validation = validation
	}
	return res, nil
}

func (s *Service) providersCheck(ctx context.Context, accounts []domain.SocialAccount, input domain.CreatePostInput) (postvalidate.Result, error) {
	return postvalidate.Check(ctx, s.providers, accounts, input)
}

// ResolveTargets verifies a set of target account IDs: they must all exist and
// belong to one team (teamID when non-empty, otherwise inferred), and every
// overrideKey must reference one of the targets so no per-account override is
// silently dropped. It returns the accounts in target order and the effective
// team. An empty targets slice is allowed (e.g. an empty draft) and yields no
// accounts and the requested team. Used by Prepare and by template/feed callers
// that have target accounts but no post body.
func (s *Service) ResolveTargets(ctx context.Context, teamID string, targets []string, overrideKeys []string) ([]domain.SocialAccount, string, error) {
	effectiveTeam := teamID
	if len(targets) == 0 {
		if len(overrideKeys) > 0 {
			return nil, "", fmt.Errorf("account_content_override references account %s which is not in target_accounts", overrideKeys[0])
		}
		return nil, effectiveTeam, nil
	}

	loaded, err := s.accounts.GetAccountsByIDsGlobal(ctx, targets)
	if err != nil {
		return nil, "", err
	}
	byID := make(map[string]domain.SocialAccount, len(loaded))
	for _, a := range loaded {
		byID[a.ID] = a
	}

	var missing []string
	for _, id := range targets {
		acc, ok := byID[id]
		if !ok {
			missing = append(missing, id)
			continue
		}
		if effectiveTeam == "" {
			effectiveTeam = acc.TeamID
			continue
		}
		if acc.TeamID != effectiveTeam {
			// A concrete team was requested -> the account is in the wrong team;
			// otherwise the targets simply span more than one team.
			if teamID != "" {
				return nil, "", fmt.Errorf("target account %s does not belong to team %s", id, teamID)
			}
			return nil, "", errors.New("target accounts must belong to one team")
		}
	}
	if len(missing) > 0 {
		return nil, "", fmt.Errorf("unknown target account(s): %s", strings.Join(missing, ", "))
	}
	for _, id := range overrideKeys {
		if _, ok := byID[id]; !ok {
			return nil, "", fmt.Errorf("account_content_override references account %s which is not in target_accounts", id)
		}
	}

	ordered := make([]domain.SocialAccount, 0, len(targets))
	for _, id := range targets {
		ordered = append(ordered, byID[id])
	}
	return ordered, effectiveTeam, nil
}

// ValidationError turns a failed character/media result into an actionable error
// for callers (e.g. MCP) that must reject rather than report. Returns nil when
// the result is valid.
func ValidationError(res postvalidate.Result) error {
	if res.Valid {
		return nil
	}
	return fmt.Errorf("content does not fit every destination: %s. Shorten the content or add account_content_override entries that fit each account's limit, then try again", res.Problems())
}
