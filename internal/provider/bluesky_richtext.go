package provider

import (
	"git.f4mily.net/goloom/internal/hashtag"
)

// blueskyFacets builds rich text facets (clickable hashtags and links) for an
// app.bsky.feed.post record. Without facets Bluesky renders plain text and
// hashtags are not indexed in tag feeds. Index offsets are UTF-8 byte offsets.
func blueskyFacets(text string) []map[string]any {
	var facets []map[string]any
	for _, m := range hashtag.Matches(text) {
		facets = append(facets, map[string]any{
			"index": map[string]any{"byteStart": m.Start, "byteEnd": m.End},
			"features": []map[string]any{{
				"$type": "app.bsky.richtext.facet#tag",
				"tag":   m.Display,
			}},
		})
	}
	for _, m := range hashtag.URLMatches(text) {
		facets = append(facets, map[string]any{
			"index": map[string]any{"byteStart": m.Start, "byteEnd": m.End},
			"features": []map[string]any{{
				"$type": "app.bsky.richtext.facet#link",
				"uri":   m.Display,
			}},
		})
	}
	return facets
}
