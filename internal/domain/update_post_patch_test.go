package domain

import (
	"testing"
	"time"
)

func TestApplyPostPatch_ScheduledAtOnlyKeepsVersionOverrides(t *testing.T) {
	existing := ScheduledPost{
		ID:             "post-1",
		Title:          "T",
		Content:        string(runeLen(419)),
		ScheduledAt:    time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC),
		TargetAccounts: []string{"bs", "masto"},
	}
	versions := []PostVersion{
		{PostID: "post-1", AccountID: "bs", Content: string(runeLen(232))},
	}
	patch := UpdatePostPatch{
		ScheduledAt: PatchField[time.Time]{Set: true, Value: time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)},
	}

	merged, flags := ApplyPostPatch(existing, versions, patch)

	if !flags.ScheduledAt {
		t.Fatal("expected scheduled_at flag")
	}
	if flags.Versions || flags.Content || flags.TargetAccounts {
		t.Fatalf("unexpected flags: %+v", flags)
	}
	if merged.Content != existing.Content {
		t.Fatal("content should be unchanged in merge")
	}
	if got := merged.AccountContentOverride["bs"]; got != versions[0].Content {
		t.Fatalf("override: got len %d want 232", len(got))
	}
	if len(merged.TargetAccounts) != 2 {
		t.Fatalf("targets: %#v", merged.TargetAccounts)
	}
}

func TestApplyPostPatch_OverridePatchReplacesVersions(t *testing.T) {
	existing := ScheduledPost{
		Content:        "default",
		TargetAccounts: []string{"a"},
	}
	versions := []PostVersion{{PostID: "p", AccountID: "a", Content: "old-override"}}
	patch := UpdatePostPatch{
		AccountContentOverride: PatchField[map[string]string]{
			Set:   true,
			Value: map[string]string{"a": "new-override"},
		},
	}

	merged, flags := ApplyPostPatch(existing, versions, patch)

	if !flags.Versions {
		t.Fatal("expected versions flag")
	}
	if merged.AccountContentOverride["a"] != "new-override" {
		t.Fatalf("override: %q", merged.AccountContentOverride["a"])
	}
}

func runeLen(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = 'x'
	}
	return string(b)
}
