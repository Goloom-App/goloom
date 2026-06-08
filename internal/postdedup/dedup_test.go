package postdedup

import (
	"testing"

	"git.f4mily.net/goloom/internal/domain"
)

func TestRedundantImportedPostIDs_matchesMastodonURIAgainstNumericID(t *testing.T) {
	t.Parallel()
	refs := []PostedTargetRef{
		{
			PostID:       "goloom-post",
			PostSource:   domain.PostSourceScheduled,
			AccountID:    "acc-1",
			RemotePostID: "https://mastodon.social/users/user/statuses/109412345678901234",
			PublishedURL: "https://mastodon.social/@user/109412345678901234",
			PublishMetadata: map[string]string{
				"uri": "https://mastodon.social/users/user/statuses/109412345678901234",
			},
		},
		{
			PostID:       "imported-post",
			PostSource:   domain.PostSourceImported,
			AccountID:    "acc-1",
			RemotePostID: "109412345678901234",
			PublishedURL: "https://mastodon.social/@user/109412345678901234",
			PublishMetadata: map[string]string{
				"uri": "https://mastodon.social/users/user/statuses/109412345678901234",
			},
		},
	}

	ids := RedundantImportedPostIDs(refs)
	if len(ids) != 1 || ids[0] != "imported-post" {
		t.Fatalf("unexpected redundant ids: %v", ids)
	}
}
