package postdedup

import (
	"encoding/json"
	"strings"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
)

// PostedTargetRef is a posted target row used for imported-post deduplication.
type PostedTargetRef struct {
	PostID          string
	PostSource      domain.PostSource
	AccountID       string
	RemotePostID    string
	PublishedURL    string
	PublishMetadata map[string]string
}

// ParsePublishMetadata decodes scheduled_post_targets.publish_metadata JSON.
func ParsePublishMetadata(raw string) map[string]string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return nil
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

// RedundantImportedPostIDs returns imported post IDs that duplicate an existing goloom-published post.
func RedundantImportedPostIDs(refs []PostedTargetRef) []string {
	canonicalByAccount := make(map[string][]PostedTargetRef)
	importedByPost := make(map[string]PostedTargetRef)

	for _, ref := range refs {
		if ref.PostSource == domain.PostSourceImported {
			importedByPost[ref.PostID] = ref
			continue
		}
		canonicalByAccount[ref.AccountID] = append(canonicalByAccount[ref.AccountID], ref)
	}

	deleteSet := make(map[string]struct{})
	for postID, imp := range importedByPost {
		for _, canon := range canonicalByAccount[imp.AccountID] {
			if provider.AuthorPostIdentifiersOverlap(
				imp.RemotePostID, imp.PublishedURL, imp.PublishMetadata,
				canon.RemotePostID, canon.PublishedURL, canon.PublishMetadata,
			) {
				deleteSet[postID] = struct{}{}
				break
			}
		}
	}

	out := make([]string, 0, len(deleteSet))
	for postID := range deleteSet {
		out = append(out, postID)
	}
	return out
}
