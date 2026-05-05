package store

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
)

const mediaDir = "data/media"

// SaveMediaFile saves the content to data/media/{teamID}/{sha256} and returns the hex SHA256.
func SaveMediaFile(teamID string, r io.Reader) (string, int64, error) {
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

// GetMediaFilePath returns the absolute-ish path for a media hash.
func GetMediaFilePath(teamID, hash string) string {
	return filepath.Join(mediaDir, teamID, hash)
}

// DeleteMediaFile removes the file from disk if it exists.
func DeleteMediaFile(teamID, hash string) error {
	return os.Remove(filepath.Join(mediaDir, teamID, hash))
}
