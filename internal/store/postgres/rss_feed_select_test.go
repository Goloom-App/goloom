package postgres

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// scanRSSFeedConfig reads 18 columns from the database row.
const rssFeedConfigScanFieldCount = 18

func TestRSSFeedConfigSelectListMatchesScannerArity(t *testing.T) {
	t.Helper()
	cols := parseRSSFeedSelectColumns(rssFeedConfigSelectList)
	if len(cols) != rssFeedConfigScanFieldCount {
		t.Fatalf("rssFeedConfigSelectList: got %d columns, scanRSSFeedConfig expects %d: %v",
			len(cols), rssFeedConfigScanFieldCount, cols)
	}
	if cols[5] != "ai_enhance_enabled" {
		t.Fatalf("column 6 must be ai_enhance_enabled, got %q", cols[5])
	}
}

func TestListActiveRSSFeedConfigSelectMatchesGetByID(t *testing.T) {
	t.Helper()
	// Regression guard: scheduler uses ListActiveRSSFeedConfigs; CRUD uses GetRSSFeedConfigByID.
	// The Postgres bug omitted ai_enhance_enabled from ListActive while scanRSSFeedConfig still expected it.
	importSrc := readStoreSource(t, "rss_import.go")
	aiSrc := readStoreSource(t, "ai.go")
	listActive := extractListActiveRSSFeedSelect(importSrc)
	getByID := extractRSSFeedSelectFromSource(aiSrc, "GetRSSFeedConfigByID")
	assertRSSFeedSelectColumnsMatch(t, "ListActiveRSSFeedConfigs vs GetRSSFeedConfigByID", listActive, getByID)
}

func TestListActiveRSSFeedConfigQueryUsesCanonicalSelectList(t *testing.T) {
	t.Helper()
	src := readStoreSource(t, "rss_import.go")
	cols := extractListActiveRSSFeedSelect(src)
	assertRSSFeedSelectColumnsMatch(t, "ListActiveRSSFeedConfigs", cols, parseRSSFeedSelectColumns(rssFeedConfigSelectList))
}

func extractListActiveRSSFeedSelect(src string) []string {
	if strings.Contains(src, "SELECT` + rssFeedConfigSelectList") {
		return parseRSSFeedSelectColumns(rssFeedConfigSelectList)
	}
	return extractRSSFeedSelectFromSource(src, "ListActiveRSSFeedConfigs")
}

func TestGetRSSFeedConfigByIDSelectMatchesCanonicalList(t *testing.T) {
	t.Helper()
	src := readStoreSource(t, "ai.go")
	cols := extractRSSFeedSelectFromSource(src, "GetRSSFeedConfigByID")
	assertRSSFeedSelectColumnsMatch(t, "GetRSSFeedConfigByID", cols, parseRSSFeedSelectColumns(rssFeedConfigSelectList))
}

func TestSQLiteRSSFeedSelectMatchesPostgresCanonicalList(t *testing.T) {
	t.Helper()
	src := readSQLiteSource(t, "ai.go")
	cols := extractRSSFeedSelectFromConst(src, "rssFeedSelectQuery")
	assertRSSFeedSelectColumnsMatch(t, "sqlite rssFeedSelectQuery", cols, parseRSSFeedSelectColumns(rssFeedConfigSelectList))
}

func assertRSSFeedSelectColumnsMatch(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s SELECT column count: got %d (%v), want %d (%v)", label, len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s SELECT column %d: got %q, want %q", label, i+1, got[i], want[i])
		}
	}
}

func parseRSSFeedSelectColumns(selectList string) []string {
	parts := strings.Split(selectList, ",")
	cols := make([]string, 0, len(parts))
	for _, part := range parts {
		col := strings.TrimSpace(part)
		if col == "" {
			continue
		}
		cols = append(cols, col)
	}
	return cols
}

var rssFeedSelectFromTable = regexp.MustCompile(`(?is)SELECT\s+(.+?)\s+FROM\s+rss_feed_configs`)

func extractRSSFeedSelectFromSource(src, funcName string) []string {
	idx := strings.Index(src, "func (s *Store) "+funcName)
	if idx < 0 {
		panic("function not found: " + funcName)
	}
	rest := src[idx:]
	match := rssFeedSelectFromTable.FindStringSubmatch(rest)
	if len(match) < 2 {
		panic("SELECT ... FROM rss_feed_configs not found in " + funcName)
	}
	selectList := strings.TrimSpace(match[1])
	if strings.Contains(selectList, "rssFeedConfigSelectList") {
		selectList = rssFeedConfigSelectList
	}
	return parseRSSFeedSelectColumns(selectList)
}

func extractRSSFeedSelectFromConst(src, constName string) []string {
	idx := strings.Index(src, "const "+constName)
	if idx < 0 {
		panic("const not found: " + constName)
	}
	rest := src[idx:]
	start := strings.Index(rest, "select ")
	if start < 0 {
		start = strings.Index(rest, "SELECT ")
	}
	if start < 0 {
		panic("select not found in " + constName)
	}
	rest = rest[start:]
	match := rssFeedSelectFromTable.FindStringSubmatch(rest)
	if len(match) < 2 {
		panic("SELECT ... FROM rss_feed_configs not found in " + constName)
	}
	return parseRSSFeedSelectColumns(match[1])
}

func readStoreSource(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("internal", "store", "postgres", name)
	data, err := os.ReadFile(repoRoot(t, path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func readSQLiteSource(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("internal", "store", "sqlite", name)
	data, err := os.ReadFile(repoRoot(t, path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func repoRoot(t *testing.T, relPath string) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, relPath)); err == nil {
			return filepath.Join(dir, relPath)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("%s not found above %s", relPath, dir)
		}
		dir = parent
	}
}
