package postgres

import (
	"strings"
	"testing"
)

// TestEmbeddedSchemaHasRequiredTables guards against the embedded postgres schema
// (applied on every startup) drifting from the tables the store writes to. The
// audit_events table was missing here once, so InsertAuditEvent failed silently
// on postgres and the team audit log stayed empty. This DB-free check catches
// that class of bug without a running database.
func TestEmbeddedSchemaHasRequiredTables(t *testing.T) {
	required := []string{
		"scheduled_posts",
		"post_versions",
		"scheduled_post_targets",
		"post_templates",
		"rss_feed_configs",
		"campaign_formats",
		"log_entries",
		"audit_events",
	}
	for _, table := range required {
		if !strings.Contains(schemaSQL, "create table if not exists "+table) {
			t.Errorf("embedded postgres schema is missing `create table if not exists %s` — inserts into it will fail at runtime", table)
		}
	}
}
