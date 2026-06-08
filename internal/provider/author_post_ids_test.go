package provider

import "testing"

func TestCollectAuthorPostIdentifiers_mastodonCrossReferences(t *testing.T) {
	t.Parallel()
	ids := CollectAuthorPostIdentifiers(
		"109412345678901234",
		"https://mastodon.social/@user/109412345678901234",
		map[string]string{"uri": "https://mastodon.social/users/user/statuses/109412345678901234"},
	)
	if len(ids) < 2 {
		t.Fatalf("expected multiple identifiers, got %v", ids)
	}
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	for _, want := range []string{
		"109412345678901234",
		"https://mastodon.social/@user/109412345678901234",
		"https://mastodon.social/users/user/statuses/109412345678901234",
	} {
		if _, ok := set[want]; !ok {
			t.Fatalf("missing identifier %q in %v", want, ids)
		}
	}
}

func TestAuthorPostIdentifiersOverlap_uriStoredOnGoloomPost(t *testing.T) {
	t.Parallel()
	overlap := AuthorPostIdentifiersOverlap(
		"https://mastodon.social/users/user/statuses/109412345678901234",
		"https://mastodon.social/@user/109412345678901234",
		map[string]string{"uri": "https://mastodon.social/users/user/statuses/109412345678901234"},
		"109412345678901234",
		"https://mastodon.social/@user/109412345678901234",
		map[string]string{"uri": "https://mastodon.social/users/user/statuses/109412345678901234"},
	)
	if !overlap {
		t.Fatal("expected goloom URI remote_post_id to match imported numeric id")
	}
}
