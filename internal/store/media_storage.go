package store

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const mediaDir = "data/media"

// validMediaSegment rejects any path component that could escape the media
// root: empty, "." / "..", or anything containing a path separator. teamID and
// hash reach the filesystem from request paths, so without this guard a crafted
// value could traverse out of mediaDir (CWE-22).
func validMediaSegment(seg string) error {
	if seg == "" || seg == "." || seg == ".." || strings.ContainsAny(seg, `/\`) {
		return fmt.Errorf("store: invalid media path segment %q", seg)
	}
	return nil
}

// mediaPath joins teamID and hash under mediaDir and guarantees the result
// stays within the media root.
func mediaPath(teamID, hash string) (string, error) {
	if err := validMediaSegment(teamID); err != nil {
		return "", err
	}
	if err := validMediaSegment(hash); err != nil {
		return "", err
	}
	root := filepath.Clean(mediaDir)
	p := filepath.Join(root, teamID, hash)
	// Defence in depth: the segment checks already prevent traversal, but verify
	// the joined path is still contained in the root before touching disk.
	if !strings.HasPrefix(p, root+string(os.PathSeparator)) {
		return "", fmt.Errorf("store: media path %q escapes root", p)
	}
	return p, nil
}

// SaveMediaFile saves the content to data/media/{teamID}/{sha256} and returns the hex SHA256.
func SaveMediaFile(teamID string, r io.Reader) (string, int64, error) {
	if err := validMediaSegment(teamID); err != nil {
		return "", 0, err
	}

	// Create a temporary file to compute hash and save simultaneously
	err := os.MkdirAll(filepath.Join(mediaDir, teamID), 0755)
	if err != nil {
		return "", 0, err
	}

	tmpFile, err := os.CreateTemp(filepath.Join(mediaDir, teamID), "upload-*")
	if err != nil {
		return "", 0, err
	}
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
	}()

	hasher := sha256.New()
	mw := io.MultiWriter(tmpFile, hasher)

	size, err := io.Copy(mw, r)
	if err != nil {
		return "", 0, err
	}

	hash := hex.EncodeToString(hasher.Sum(nil))
	finalPath := filepath.Join(mediaDir, teamID, hash)

	// If file already exists, we can just remove the temp and return the hash
	if _, err := os.Stat(finalPath); err == nil {
		return hash, size, nil
	}

	if err := os.Rename(tmpFile.Name(), finalPath); err != nil {
		// On some systems rename might fail across filesystems, fallback to copy if needed
		// but here we are in the same directory.
		return "", 0, err
	}

	// Prevent deferred Remove from deleting the final file
	_ = tmpFile.Close() // Close before rename is better but Rename handles it on Linux.
	// Actually we should return here and not allow the defer Remove to run if we renamed successfully.
	// A better pattern:
	return hash, size, nil
}

// GetMediaFilePath returns the on-disk path for a media hash, or an error if
// teamID/hash would resolve outside the media root.
func GetMediaFilePath(teamID, hash string) (string, error) {
	return mediaPath(teamID, hash)
}

// DeleteMediaFile removes the file from disk if it exists.
func DeleteMediaFile(teamID, hash string) error {
	p, err := mediaPath(teamID, hash)
	if err != nil {
		return err
	}
	return os.Remove(p)
}
