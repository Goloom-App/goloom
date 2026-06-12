package ai

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// params is the loosely-typed job parameter object carried in AIJob payloads.
type params map[string]any

func parseJobParams(payload json.RawMessage) (params, error) {
	if len(payload) == 0 {
		return params{}, nil
	}
	var envelope struct {
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, fmt.Errorf("parse job payload: %w", err)
	}
	if len(envelope.Params) == 0 {
		return params{}, nil
	}
	var out params
	if err := json.Unmarshal(envelope.Params, &out); err != nil {
		return nil, fmt.Errorf("parse job params: %w", err)
	}
	if out == nil {
		out = params{}
	}
	return out, nil
}

// str returns the first non-empty string value among the given keys.
func (p params) str(keys ...string) string {
	for _, key := range keys {
		if value, ok := p[key]; ok {
			if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func (p params) boolean(keys ...string) bool {
	for _, key := range keys {
		if value, ok := p[key]; ok {
			if b, ok := value.(bool); ok {
				return b
			}
		}
	}
	return false
}

func (p params) intval(key string, fallback int) int {
	if value, ok := p[key]; ok {
		switch v := value.(type) {
		case float64:
			return int(v)
		case int:
			return v
		case string:
			var n int
			if _, err := fmt.Sscanf(strings.TrimSpace(v), "%d", &n); err == nil {
				return n
			}
		}
	}
	return fallback
}

func (p params) stringList(keys ...string) []string {
	for _, key := range keys {
		value, ok := p[key]
		if !ok || value == nil {
			continue
		}
		switch v := value.(type) {
		case []any:
			var out []string
			for _, item := range v {
				s := strings.TrimSpace(fmt.Sprintf("%v", item))
				if s != "" {
					out = append(out, s)
				}
			}
			return out
		case []string:
			return v
		case string:
			if strings.TrimSpace(v) != "" {
				return []string{strings.TrimSpace(v)}
			}
		}
	}
	return nil
}

// nested returns a nested params object under one of the given keys.
func (p params) nested(keys ...string) params {
	for _, key := range keys {
		if value, ok := p[key]; ok {
			if m, ok := value.(map[string]any); ok {
				return params(m)
			}
		}
	}
	return nil
}

// formatValue renders an arbitrary param value deterministically for prompts.
func formatValue(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case map[string]any, []any:
		encoded, err := marshalSorted(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return encoded
	default:
		return fmt.Sprintf("%v", v)
	}
}

// marshalSorted encodes maps with sorted keys (matches Python's sort_keys=True).
func marshalSorted(value any) (string, error) {
	switch v := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var sb strings.Builder
		sb.WriteString("{")
		for i, key := range keys {
			if i > 0 {
				sb.WriteString(", ")
			}
			keyJSON, _ := json.Marshal(key)
			sb.Write(keyJSON)
			sb.WriteString(": ")
			inner, err := marshalSorted(v[key])
			if err != nil {
				return "", err
			}
			sb.WriteString(inner)
		}
		sb.WriteString("}")
		return sb.String(), nil
	case []any:
		var sb strings.Builder
		sb.WriteString("[")
		for i, item := range v {
			if i > 0 {
				sb.WriteString(", ")
			}
			inner, err := marshalSorted(item)
			if err != nil {
				return "", err
			}
			sb.WriteString(inner)
		}
		sb.WriteString("]")
		return sb.String(), nil
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(encoded), nil
	}
}
