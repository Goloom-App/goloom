package store

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestGetMediaFilePathContainsWithinRoot(t *testing.T) {
	t.Parallel()

	root := filepath.Clean(mediaDir)

	t.Run("valid segments resolve under the media root", func(t *testing.T) {
		t.Parallel()
		got, err := GetMediaFilePath("team123", "abcdef0123456789")
		if err != nil {
			t.Fatalf("GetMediaFilePath returned unexpected error: %v", err)
		}
		want := filepath.Join(root, "team123", "abcdef0123456789")
		if got != want {
			t.Fatalf("GetMediaFilePath = %q, want %q", got, want)
		}
	})

	traversal := []struct {
		name   string
		teamID string
		hash   string
	}{
		{"parent in teamID", "..", "hash"},
		{"separator in teamID", "../../etc", "passwd"},
		{"backslash in teamID", `..\..\secret`, "hash"},
		{"empty teamID", "", "hash"},
		{"dot teamID", ".", "hash"},
		{"separator in hash", "team", "../escape"},
		{"parent in hash", "team", ".."},
		{"empty hash", "team", ""},
	}
	for _, tc := range traversal {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := GetMediaFilePath(tc.teamID, tc.hash)
			if err == nil {
				t.Fatalf("GetMediaFilePath(%q, %q) = %q, want error", tc.teamID, tc.hash, got)
			}
			if got != "" {
				t.Fatalf("GetMediaFilePath(%q, %q) returned path %q alongside error", tc.teamID, tc.hash, got)
			}
		})
	}
}

func TestDeleteMediaFileRejectsTraversal(t *testing.T) {
	t.Parallel()
	if err := DeleteMediaFile("../../etc", "passwd"); err == nil {
		t.Fatal("DeleteMediaFile accepted a traversing path, want error")
	}
}

func TestSaveMediaFileRejectsTraversalTeamID(t *testing.T) {
	t.Parallel()
	_, _, err := SaveMediaFile("../escape", strings.NewReader("data"))
	if err == nil {
		t.Fatal("SaveMediaFile accepted a traversing teamID, want error")
	}
}
