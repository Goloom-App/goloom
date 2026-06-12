package ai

import (
	"encoding/json"
	"fmt"
	"strings"
)

// extractJSONObject tolerantly parses an LLM response into a JSON object,
// stripping code fences and surrounding prose if necessary.
func extractJSONObject(raw string) (map[string]any, error) {
	cleaned := strings.TrimSpace(raw)
	if strings.HasPrefix(cleaned, "```") {
		if idx := strings.Index(cleaned, "\n"); idx != -1 {
			cleaned = cleaned[idx+1:]
		}
		if idx := strings.LastIndex(cleaned, "```"); idx != -1 {
			cleaned = cleaned[:idx]
		}
		cleaned = strings.TrimSpace(cleaned)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(cleaned), &payload); err == nil {
		return payload, nil
	}

	start := strings.Index(cleaned, "{")
	end := strings.LastIndex(cleaned, "}")
	if start != -1 && end > start {
		if err := json.Unmarshal([]byte(cleaned[start:end+1]), &payload); err == nil {
			return payload, nil
		}
	}
	return nil, fmt.Errorf("LLM response was not valid JSON")
}

func payloadString(payload map[string]any, key string) string {
	if value, ok := payload[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func payloadStringList(payload map[string]any, key string) []string {
	value, ok := payload[key]
	if !ok || value == nil {
		return nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, item := range items {
		s := strings.TrimSpace(fmt.Sprintf("%v", item))
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func payloadObject(payload map[string]any, key string) map[string]any {
	if value, ok := payload[key].(map[string]any); ok {
		return value
	}
	return map[string]any{}
}
