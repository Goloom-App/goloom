package domain

import (
	"path/filepath"
	"strings"
)

// Log component labels identify the area of the codebase that emitted a log
// entry. They are derived from the entry's source file path so operators can
// filter the admin log view down to AI, MCP, automation, etc.
const (
	LogComponentAI         = "ai"
	LogComponentMCP        = "mcp"
	LogComponentAutomation = "automation"
	LogComponentProvider   = "provider"
	LogComponentAPI        = "api"
	LogComponentSystem     = "system"
)

// logComponentRules maps a component to the source-path fragments that identify
// it. Order matters: the first matching rule wins. This is the single source of
// truth shared by LogComponentFromSource (display) and the store query builders
// (filtering), so derivation and filtering can never drift apart.
var logComponentRules = []struct {
	Component string
	Fragments []string
}{
	{LogComponentAI, []string{"/internal/ai/", "/internal/aijobs/"}},
	{LogComponentMCP, []string{"/internal/mcp/"}},
	{LogComponentAutomation, []string{"/internal/scheduler/"}},
	{LogComponentProvider, []string{"/internal/provider/"}},
	{LogComponentAPI, []string{"/api/"}},
}

// LogComponents returns the selectable component filters, in display order. The
// catch-all "system" bucket is listed last.
func LogComponents() []string {
	out := make([]string, 0, len(logComponentRules)+1)
	for _, r := range logComponentRules {
		out = append(out, r.Component)
	}
	return append(out, LogComponentSystem)
}

// LogComponentFromSource derives the component label for a source file path.
// Anything that matches no known area is reported as "system".
func LogComponentFromSource(sourceFile string) string {
	s := filepath.ToSlash(sourceFile)
	for _, r := range logComponentRules {
		for _, frag := range r.Fragments {
			if strings.Contains(s, frag) {
				return r.Component
			}
		}
	}
	return LogComponentSystem
}

// LogComponentFilter returns the source-path fragments used to filter logs to a
// component. When negate is true (the "system" bucket), entries match when they
// contain none of the fragments. An empty fragment slice means "no constraint".
func LogComponentFilter(component string) (fragments []string, negate bool) {
	if component == LogComponentSystem {
		var all []string
		for _, r := range logComponentRules {
			all = append(all, r.Fragments...)
		}
		return all, true
	}
	for _, r := range logComponentRules {
		if r.Component == component {
			return r.Fragments, false
		}
	}
	return nil, false
}
