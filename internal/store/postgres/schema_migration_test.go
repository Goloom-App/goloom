package postgres

import (
	"strings"
	"testing"
)

func TestScheduledPostsSourceCheckAllowsAutomation(t *testing.T) {
	t.Helper()
	if strings.Contains(schemaSQL, "check (source in ('scheduled', 'imported'))") {
		t.Fatal("schema must not re-apply a narrow scheduled_posts source check without automation on startup")
	}
	if !strings.Contains(schemaSQL, "check (source in ('scheduled', 'imported', 'automation'))") {
		t.Fatal("schema must enforce scheduled_posts source check including automation")
	}
	if strings.Count(schemaSQL, "add constraint scheduled_posts_source_check") != 1 {
		t.Fatalf("expected exactly one scheduled_posts_source_check add, got %d",
			strings.Count(schemaSQL, "add constraint scheduled_posts_source_check"))
	}
}
