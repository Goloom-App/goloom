package agenttools

import (
	"context"
	"fmt"
	"strings"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/postvalidate"
)

// coreReviseComposerPost proposes a revision for the post the user is editing in
// the composer. It does NOT persist anything: it validates the proposed text
// against the team's character limits and returns it so the chat UI can apply it
// to the open (unsaved) composer draft. The assistant reaches for this — instead
// of draft_post/modify_post — when the current view is the composer, because that
// draft has no post id to patch.
func coreReviseComposerPost(ctx context.Context, d Deps, inv Invocation, in ReviseComposerPostInput) (ReviseComposerPostOutput, error) {
	if err := requireTeam(ctx, d, inv, in.TeamID, roleViewer...); err != nil {
		return ReviseComposerPostOutput{}, err
	}

	accounts, err := d.Store.ListTeamAccounts(ctx, in.TeamID)
	if err != nil {
		return ReviseComposerPostOutput{}, err
	}
	limitByID := make(map[string]postvalidate.AccountLimit, len(accounts))
	allIDs := make([]string, 0, len(accounts))
	for _, account := range accounts {
		allIDs = append(allIDs, account.ID)
		limitByID[account.ID] = postvalidate.AccountLimit{
			AccountID: account.ID,
			Username:  account.Username,
			Provider:  account.Provider,
			MaxChars:  domain.MaxCharsForProvider(account.Provider, account.MaxCharsOverride),
		}
	}

	// Reject override ids that are not connected accounts before normalisation
	// drops them silently.
	for id := range in.AccountContentOverride {
		if _, ok := limitByID[id]; !ok {
			return ReviseComposerPostOutput{}, fmt.Errorf("unknown account id %q: use the connected account ids from the system context", id)
		}
	}

	content := strings.TrimSpace(in.Content)
	overrides := domain.NormalizeAccountContentOverride(in.AccountContentOverride, allIDs)
	if content == "" && len(overrides) == 0 {
		return ReviseComposerPostOutput{}, fmt.Errorf("provide content and/or account_content_override")
	}

	// A new default text must fit every account that would use it; a per-account
	// override is checked against its own account's limit only.
	var targetIDs []string
	if content != "" {
		targetIDs = allIDs
	} else {
		for id := range overrides {
			targetIDs = append(targetIDs, id)
		}
	}
	limits := make([]postvalidate.AccountLimit, 0, len(targetIDs))
	for _, id := range targetIDs {
		if l, ok := limitByID[id]; ok && l.MaxChars > 0 {
			limits = append(limits, l)
		}
	}
	if res := postvalidate.CheckLimits(content, overrides, limits); !res.Valid {
		return ReviseComposerPostOutput{}, fmt.Errorf("character limit exceeded: %s. Shorten the text or add per-account overrides that fit each account's limit", res.Problems())
	}

	if overrides == nil {
		overrides = map[string]string{}
	}
	return ReviseComposerPostOutput{Content: content, AccountContentOverride: overrides}, nil
}
