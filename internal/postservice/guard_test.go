package postservice

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// automationBypassAllowlist lists the non-interactive create paths that
// legitimately build a post without the full Prepare pipeline: automation and
// AI flows that supply their own targets and call domain.EnsureTitle. Every
// other caller of store.CreateScheduledPost must go through postservice.Prepare,
// so a new interactive endpoint cannot silently skip validation.
var automationBypassAllowlist = map[string]bool{
	"api/ai_draft.go":                      true,
	"api/ai_callback.go":                   true,
	"api/ai_chat.go":                       true,
	"api/rss_automation_callback.go":       true,
	"api/recurring_automation_callback.go": true,
	"api/review_queue.go":                  true,
	"internal/scheduler/scheduler.go":      true,
	"internal/scheduler/rss_ai.go":         true,
}

// TestNoUnvalidatedPostCreation guards the invariant established by the post
// pipeline refactor: interactive callers must validate via Prepare before
// persisting. It fails if a new file calls store.CreateScheduledPost without
// running the pipeline and without being explicitly allowlisted as automation.
func TestNoUnvalidatedPostCreation(t *testing.T) {
	root := repoRoot(t)
	var offenders []string

	for _, dir := range []string{"api", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			// The store package defines and implements CreateScheduledPost; it is
			// the persistence layer, not a caller to gate.
			rel, _ := filepath.Rel(root, path)
			if strings.HasPrefix(rel, "internal/store/") || strings.HasPrefix(rel, "internal/postservice/") {
				return nil
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := string(src)
			if !strings.Contains(text, ".CreateScheduledPost(") {
				return nil
			}
			if automationBypassAllowlist[filepath.ToSlash(rel)] {
				return nil
			}
			if !strings.Contains(text, ".Prepare(") {
				offenders = append(offenders, rel)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	if len(offenders) > 0 {
		t.Fatalf("these files create posts without the postservice pipeline (call posts.Prepare or add to the automation allowlist): %v", offenders)
	}
}

// repoRoot walks up from the test's working directory to the module root.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found walking up from test dir")
		}
		dir = parent
	}
}
