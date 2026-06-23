package mcp

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/postvalidate"
	"git.f4mily.net/goloom/internal/provider"
)

// validateFeedURL rejects empty or non-http(s) RSS feed URLs before they are
// persisted, so a feed automation can never be created with an unusable source.
func validateFeedURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("feed_url is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("feed_url is not a valid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("feed_url must be an http(s) URL")
	}
	if u.Host == "" {
		return fmt.Errorf("feed_url must include a host")
	}
	return nil
}

// resolveTargets loads the target accounts and enforces the invariants every
// post-creating MCP tool needs: target_accounts must be non-empty, every id must
// exist and belong to teamID (preventing cross-team targeting), and every
// account_content_override key must reference one of the targets so overrides are
// never silently dropped. It returns the accounts in the caller's target order.
func (h *Handler) resolveTargets(ctx context.Context, teamID string, targets []string, overrides map[string]string) ([]domain.SocialAccount, error) {
	if len(targets) == 0 {
		return nil, fmt.Errorf("target_accounts is required")
	}
	accounts, err := h.store.GetAccountsByIDsGlobal(ctx, targets)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]domain.SocialAccount, len(accounts))
	for _, a := range accounts {
		byID[a.ID] = a
	}

	var missing []string
	for _, id := range targets {
		acc, ok := byID[id]
		if !ok {
			missing = append(missing, id)
			continue
		}
		if acc.TeamID != teamID {
			return nil, fmt.Errorf("target account %s does not belong to team %s", id, teamID)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("unknown target account(s): %s", strings.Join(missing, ", "))
	}

	for id, content := range overrides {
		if strings.TrimSpace(content) == "" {
			continue
		}
		if _, ok := byID[id]; !ok {
			return nil, fmt.Errorf("account_content_override references account %s which is not in target_accounts", id)
		}
	}

	ordered := make([]domain.SocialAccount, 0, len(targets))
	for _, id := range targets {
		ordered = append(ordered, byID[id])
	}
	return ordered, nil
}

// enforceCharLimits rejects content that exceeds any destination's character
// limit (or omits required media), with a message an agent can act on.
func enforceCharLimits(ctx context.Context, providers *provider.Registry, accounts []domain.SocialAccount, input domain.CreatePostInput) error {
	res, err := postvalidate.Check(ctx, providers, accounts, input)
	if err != nil {
		return err
	}
	if !res.Valid {
		return fmt.Errorf("content does not fit every destination: %s. Shorten the content or add account_content_override entries that fit each account's limit, then call the tool again", res.Problems())
	}
	return nil
}
