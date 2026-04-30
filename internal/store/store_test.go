package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.f4mily.net/goloom/internal/security"
)

func TestIsPostgresURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw  string
		want bool
	}{
		{"", false},
		{"   ", false},
		{"postgres://localhost/db", true},
		{"POSTGRESQL://localhost/db", true},
		{"postgresql://user:pass@host:5432/name", true},
		{"postgres://", true},
		{":memory:", false},
		{"file:./data.db", false},
		{"sqlite://./x.db", false},
		{"https://example.com", false},
		{"http://postgres:5432", false}, // scheme http, not postgres
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			t.Parallel()
			if got := isPostgresURL(tc.raw); got != tc.want {
				t.Errorf("isPostgresURL(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestIsPostgresURL_parseURLScheme(t *testing.T) {
	t.Parallel()
	// URL with uppercase scheme via generic parse path
	if !isPostgresURL("postgres://host/db?sslmode=disable") {
		t.Error("expected true")
	}
}

func TestNormalizeSQLiteURL(t *testing.T) {
	tmp := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		raw     string
		want    string
		wantErr bool
	}{
		{"", "file:./data/goloom.db", false},
		{"  ", "file:./data/goloom.db", false},
		{":memory:", ":memory:", false},
		{"sqlite://", "file:./data/goloom.db", false},
		{"sqlite:///tmp/foo.db", "file:/tmp/foo.db", false},
		{"sqlite://relative/path.db", "file:relative/path.db", false},
		{"sqlite:./plain.db", "file:./plain.db", false},
		{"./direct.db", "file:./direct.db", false},
		{"file:./x.db", "file:./x.db", false},
	}
	for _, tc := range cases {
		name := tc.raw
		if strings.TrimSpace(name) == "" {
			name = "(empty)"
		}
		t.Run(name, func(t *testing.T) {
			got, err := normalizeSQLiteURL(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNormalizeSQLiteURL_absoluteFilePath(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "nested", "app.db")
	got, err := normalizeSQLiteURL(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	want := "file:" + filepath.Join(tmp, "nested", "app.db")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(tmp, "nested")); err != nil {
		t.Fatalf("parent dir should exist: %v", err)
	}
}

func TestEnsureSQLiteParentDir_skipsMemoryAndRoot(t *testing.T) {
	t.Parallel()
	for _, dsn := range []string{":memory:", "file::memory:?cache=shared", "file:x.db", "file:/abs.db"} {
		if err := ensureSQLiteParentDir(dsn); err != nil {
			t.Errorf("ensureSQLiteParentDir(%q): %v", dsn, err)
		}
	}
}

func TestOpen_sqliteMemory(t *testing.T) {
	t.Parallel()
	enc, err := security.NewEncrypter("open-test-secret-string-here")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	s, err := Open(ctx, ":memory:", enc)
	if err != nil {
		t.Fatalf("Open :memory: %v", err)
	}
	defer s.Close()
}

func TestOpen_sqliteSharedMemoryDSN(t *testing.T) {
	t.Parallel()
	enc, _ := security.NewEncrypter("open-test-secret-string-here")
	ctx := context.Background()
	s, err := Open(ctx, "file::memory:?cache=shared", enc)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
}
