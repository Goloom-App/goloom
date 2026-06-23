package mcp

import (
	"fmt"
	"net/url"
	"strings"
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
