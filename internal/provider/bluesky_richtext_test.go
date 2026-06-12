package provider

import (
	"testing"
)

func TestBlueskyFacets(t *testing.T) {
	text := "Grüße! #GoLoom Infos: https://example.com/x"
	facets := blueskyFacets(text)
	if len(facets) != 2 {
		t.Fatalf("got %d facets, want 2", len(facets))
	}

	tagIdx := facets[0]["index"].(map[string]any)
	start, end := tagIdx["byteStart"].(int), tagIdx["byteEnd"].(int)
	if text[start:end] != "#GoLoom" {
		t.Fatalf("tag facet selects %q, want %q", text[start:end], "#GoLoom")
	}
	tagFeat := facets[0]["features"].([]map[string]any)[0]
	if tagFeat["$type"] != "app.bsky.richtext.facet#tag" || tagFeat["tag"] != "GoLoom" {
		t.Fatalf("unexpected tag feature: %#v", tagFeat)
	}

	linkFeat := facets[1]["features"].([]map[string]any)[0]
	if linkFeat["$type"] != "app.bsky.richtext.facet#link" || linkFeat["uri"] != "https://example.com/x" {
		t.Fatalf("unexpected link feature: %#v", linkFeat)
	}
}

func TestBlueskyFacetsEmpty(t *testing.T) {
	if facets := blueskyFacets("nur text"); facets != nil {
		t.Fatalf("expected nil facets, got %#v", facets)
	}
}
