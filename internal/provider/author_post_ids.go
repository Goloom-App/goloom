package provider

import (
	"net/url"
	"strings"
	"unicode"
)

// CollectAuthorPostIdentifiers returns every provider-specific identifier that may
// refer to the same published post (numeric status id, canonical URI, public URL, …).
func CollectAuthorPostIdentifiers(remoteID, publishedURL string, metadata map[string]string) []string {
	seen := make(map[string]struct{})
	add := func(value string) {
		for _, id := range expandAuthorPostReference(value) {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			seen[id] = struct{}{}
		}
	}

	add(remoteID)
	add(publishedURL)
	if metadata != nil {
		add(metadata["uri"])
	}

	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	return out
}

func expandAuthorPostReference(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	ids := []string{value}
	if isAllDigits(value) {
		return ids
	}
	if statusID := mastodonStatusIDFromReference(value); statusID != "" {
		ids = append(ids, statusID)
	}
	return ids
}

func mastodonStatusIDFromReference(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if isAllDigits(ref) {
		return ref
	}
	if idx := strings.Index(ref, "/statuses/"); idx >= 0 {
		id := strings.SplitN(ref[idx+len("/statuses/"):], "/", 2)[0]
		if isAllDigits(id) {
			return id
		}
	}
	if parsed, err := url.Parse(ref); err == nil {
		segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		if len(segments) > 0 {
			last := segments[len(segments)-1]
			if isAllDigits(last) {
				return last
			}
		}
	}
	return ""
}

func isAllDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// AuthorPostIdentifiersOverlap reports whether two posted targets refer to the same remote post.
func AuthorPostIdentifiersOverlap(
	leftRemoteID, leftPublishedURL string, leftMetadata map[string]string,
	rightRemoteID, rightPublishedURL string, rightMetadata map[string]string,
) bool {
	left := CollectAuthorPostIdentifiers(leftRemoteID, leftPublishedURL, leftMetadata)
	rightSet := make(map[string]struct{}, len(left))
	for _, id := range CollectAuthorPostIdentifiers(rightRemoteID, rightPublishedURL, rightMetadata) {
		rightSet[id] = struct{}{}
	}
	for _, id := range left {
		if _, ok := rightSet[id]; ok {
			return true
		}
	}
	return false
}
