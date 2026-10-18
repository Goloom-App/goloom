package sqlite

import "encoding/json"

// Preserve null (inherit) versus [] (none); IDs are validated by the API and
// context builder, not through foreign keys so deleted selections remain visible.
func knowledgeSelectionJSON(ids []string) string {
	raw, _ := json.Marshal(ids) // []string always marshals successfully
	return string(raw)
}
