package postservice

import (
	"context"
	"strings"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
)

// TestContract is the single, readable specification of what makes a post valid.
// Because every interactive create/update path (REST and MCP) runs through
// Prepare, asserting the rules here once guarantees both transports agree.
func TestContract(t *testing.T) {
	const team = "team-1"
	svc := newService(
		acc("bsky", "bluesky", team),   // 300-char limit
		acc("masto", "mastodon", team), // 500-char limit
		acc("foreign", "mastodon", "team-2"),
	)

	long := strings.Repeat("x", 400) // > 300 (bluesky), < 500 (mastodon)

	cases := []struct {
		name        string
		input       domain.CreatePostInput
		opts        Options
		wantErr     bool   // hard rejection
		wantInvalid bool   // soft char/media failure (Validation.Valid == false)
		errContains string // substring of the hard error, when wantErr
	}{
		{
			name:  "valid scheduled post",
			input: base("hello", "bsky", "masto"),
			opts:  Options{CheckLimits: true},
		},
		{
			name:    "missing title",
			input:   domain.CreatePostInput{Content: "c", TargetAccounts: []string{"bsky"}},
			opts:    Options{CheckLimits: true},
			wantErr: true, errContains: "title",
		},
		{
			name:    "scheduled missing content",
			input:   domain.CreatePostInput{Title: "T", TargetAccounts: []string{"bsky"}},
			opts:    Options{CheckLimits: true},
			wantErr: true, errContains: "content",
		},
		{
			name:    "scheduled missing targets",
			input:   domain.CreatePostInput{Title: "T", Content: "c"},
			opts:    Options{CheckLimits: true},
			wantErr: true, errContains: "target",
		},
		{
			name:    "unknown target",
			input:   base("c", "ghost"),
			opts:    Options{CheckLimits: true},
			wantErr: true, errContains: "unknown target",
		},
		{
			name:    "cross-team target",
			input:   base("c", "foreign"),
			opts:    Options{CheckLimits: true},
			wantErr: true, errContains: "does not belong to team",
		},
		{
			name: "override for non-target account",
			input: func() domain.CreatePostInput {
				in := base("c", "bsky")
				in.AccountContentOverride = map[string]string{"masto": "x"}
				return in
			}(),
			opts:    Options{CheckLimits: true},
			wantErr: true, errContains: "account_content_override",
		},
		{
			name:        "content exceeds bluesky",
			input:       base(long, "bsky", "masto"),
			opts:        Options{CheckLimits: true},
			wantInvalid: true,
		},
		{
			name: "override brings bluesky within limit",
			input: func() domain.CreatePostInput {
				in := base(long, "bsky", "masto")
				in.AccountContentOverride = map[string]string{"bsky": "short"}
				return in
			}(),
			opts: Options{CheckLimits: true},
		},
		{
			name: "oversized draft allowed (limits skipped)",
			input: func() domain.CreatePostInput {
				in := base(long, "bsky")
				in.Draft = true
				return in
			}(),
			opts: Options{CheckLimits: false},
		},
		{
			name: "draft still requires title",
			input: func() domain.CreatePostInput {
				in := domain.CreatePostInput{Content: "c", Draft: true}
				return in
			}(),
			opts:    Options{CheckLimits: false},
			wantErr: true, errContains: "title",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := svc.Prepare(context.Background(), team, tc.input, tc.opts)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected hard error, got none")
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Fatalf("error %q does not contain %q", err, tc.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected hard error: %v", err)
			}
			if res.Validation.Valid == tc.wantInvalid {
				t.Fatalf("Validation.Valid = %v, wantInvalid = %v (problems: %q)", res.Validation.Valid, tc.wantInvalid, res.Validation.Problems())
			}
		})
	}
}
