package provider

import (
	"strings"
	"testing"
)

func TestEncodeDecodeBlueskyMediaID_roundTrip(t *testing.T) {
	t.Parallel()
	blob := map[string]any{
		"$type":    "blob",
		"ref":      map[string]any{"$link": "bafyreiexample"},
		"mimeType": "image/png",
		"size":     float64(12),
	}
	id, err := encodeBlueskyMediaID(blob, "my alt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(id, blueskyMediaIDPrefix) {
		t.Fatalf("prefix: %q", id)
	}
	decoded, err := decodeBlueskyMediaIDs([]string{id})
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 1 || decoded[0].Alt != "my alt" {
		t.Fatalf("decoded: %#v", decoded)
	}
	if decoded[0].Blob["mimeType"] != "image/png" {
		t.Fatalf("blob: %#v", decoded[0].Blob)
	}
}
