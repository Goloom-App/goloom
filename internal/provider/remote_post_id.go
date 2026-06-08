package provider

import (
	"strings"
)

// ResolveRemotePostID returns the canonical provider post identifier for deduplication.
func ResolveRemotePostID(providerName string, result PublishResult) string {
	providerName = strings.ToLower(strings.TrimSpace(providerName))
	if providerName == "bluesky" {
		if uri := strings.TrimSpace(result.Metadata["uri"]); uri != "" {
			return uri
		}
	}
	return strings.TrimSpace(result.RemoteID)
}
