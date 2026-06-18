package ai

import (
	"regexp"
	"strings"
)

// PII guard (catalog §8.2): user-supplied free text may carry secrets that must
// not be forwarded to an external LLM. We redact a conservative set of patterns
// before the text enters a prompt and report which kinds were found, so the
// caller can warn the user instead of silently passing them through.

type piiPattern struct {
	kind string
	re   *regexp.Regexp
}

// Order matters: labelled secrets (password: …, api_key=…) are redacted first so
// the whole value disappears before the narrower email/key patterns run.
var piiPatterns = []piiPattern{
	{"credential", regexp.MustCompile(`(?i)\b(password|passwort|passwd|pwd|secret|api[_-]?key|token)\b\s*[:=]\s*\S+`)},
	{"api_key", regexp.MustCompile(`(?i)\b(sk|pk|rk)-[a-z0-9]{16,}\b`)},
	{"bearer_token", regexp.MustCompile(`(?i)bearer\s+[a-z0-9._\-]{12,}`)},
	{"email", regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)},
}

const piiRedaction = "[redacted]"

// redactPII replaces any PII matches in text with a placeholder and returns the
// cleaned text plus the de-duplicated kinds that were found.
func redactPII(text string) (string, []string) {
	if strings.TrimSpace(text) == "" {
		return text, nil
	}
	seen := map[string]bool{}
	var kinds []string
	cleaned := text
	for _, p := range piiPatterns {
		if !p.re.MatchString(cleaned) {
			continue
		}
		if !seen[p.kind] {
			seen[p.kind] = true
			kinds = append(kinds, p.kind)
		}
		cleaned = p.re.ReplaceAllString(cleaned, piiRedaction)
	}
	return cleaned, kinds
}

// piiUserTextKeys are the job-param keys carrying free text a user typed; these
// are scanned and redacted in place before prompt assembly.
var piiUserTextKeys = []string{
	"occasion",
	"prompt_hint",
	"content_hint",
	"request",
	"prompt",
	"instruction",
	"brief",
	"description",
	"title_hint",
}

// redactParamsPII redacts PII in the user-text fields of p in place and returns
// the de-duplicated kinds found across them.
func redactParamsPII(p params) []string {
	seen := map[string]bool{}
	var kinds []string
	for _, key := range piiUserTextKeys {
		raw, ok := p[key].(string)
		if !ok || strings.TrimSpace(raw) == "" {
			continue
		}
		cleaned, found := redactPII(raw)
		if len(found) == 0 {
			continue
		}
		p[key] = cleaned
		for _, k := range found {
			if !seen[k] {
				seen[k] = true
				kinds = append(kinds, k)
			}
		}
	}
	return kinds
}
