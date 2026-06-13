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

	// Isolate the first balanced { ... } object so prose on either side — even
	// prose that itself contains braces — does not derail parsing.
	if obj := firstJSONObject(cleaned); obj != "" {
		if err := json.Unmarshal([]byte(obj), &payload); err == nil {
			return payload, nil
		}
	}
	return nil, fmt.Errorf("LLM response was not valid JSON")
}

// firstJSONObject returns the substring spanning the first top-level JSON object
// in s by tracking brace depth while ignoring braces inside string literals. It
// returns "" if no balanced object is present (e.g. the text was truncated).
func firstJSONObject(s string) string {
	start := strings.IndexByte(s, '{')
	if start == -1 {
		return ""
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		ch := s[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case ch == '\\':
				escaped = true
			case ch == '"':
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
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
