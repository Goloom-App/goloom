package postgres

import "encoding/json"

// Preserve null (inherit) versus [] (none), including deleted source IDs.
func knowledgeSelectionJSON(ids []string) string {
	raw, _ := json.Marshal(ids) // []string always marshals successfully
	return string(raw)
}
