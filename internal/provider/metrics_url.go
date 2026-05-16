package provider

import (
	"strings"

	"git.f4mily.net/goloom/internal/domain"
)

// MetricsPublishedURL returns the URL or AT-URI the provider should use when fetching post engagement.
func MetricsPublishedURL(account domain.SocialAccount, publishedURL string, publishMetadata map[string]string) string {
	raw := strings.TrimSpace(publishedURL)
	if account.Provider == "bluesky" {
		if uri := strings.TrimSpace(publishMetadata["uri"]); uri != "" {
			return uri
		}
	}
	return raw
}
