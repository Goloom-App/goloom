package ai

import (
	"strings"
	"testing"
)

func TestRedactPII(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		wantKind  string
		mustRedax string // substring that must no longer appear
	}{
		{"email", "reach me at jane.doe@example.com please", "email", "jane.doe@example.com"},
		{"labelled password", "the password: hunter2!", "credential", "hunter2!"},
		{"openai key", "key sk-ABCDEFGHIJKLMNOP12345 here", "api_key", "sk-ABCDEFGHIJKLMNOP12345"},
		{"bearer", "Authorization: Bearer abcdef123456ghijkl", "bearer_token", "abcdef123456ghijkl"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cleaned, kinds := redactPII(tc.in)
			if strings.Contains(cleaned, tc.mustRedax) {
				t.Fatalf("secret not redacted: %q", cleaned)
			}
			found := false
			for _, k := range kinds {
				if k == tc.wantKind {
					found = true
				}
			}
			if !found {
				t.Fatalf("kind %q not reported, got %v", tc.wantKind, kinds)
			}
		})
	}
}

func TestRedactPIILeavesCleanTextUntouched(t *testing.T) {
	in := "Announce our spring meetup on May 3rd in Berlin."
	cleaned, kinds := redactPII(in)
	if cleaned != in || len(kinds) != 0 {
		t.Fatalf("clean text altered: %q kinds=%v", cleaned, kinds)
	}
}

func TestRedactParamsPIIMutatesAndReports(t *testing.T) {
	p := params{
		"prompt_hint": "post about our launch, login is admin@corp.com",
		"occasion":    "spring sale",
	}
	kinds := redactParamsPII(p)
	if len(kinds) != 1 || kinds[0] != "email" {
		t.Fatalf("expected one email kind, got %v", kinds)
	}
	if strings.Contains(p.str("prompt_hint"), "admin@corp.com") {
		t.Fatalf("param not redacted in place: %q", p.str("prompt_hint"))
	}
	if p.str("occasion") != "spring sale" {
		t.Fatal("clean param should be untouched")
	}
}
