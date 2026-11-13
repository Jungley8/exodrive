package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

var symlinkSeq uint64

// IsSymlink returns true if path exists and is a symbolic link.
func IsSymlink(path string) bool {
	fi, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeSymlink != 0
}

// ReadSymlinkTarget returns the destination of a symlink, or empty string if not a symlink.
func ReadSymlinkTarget(path string) string {
	target, err := os.Readlink(path)
	if err != nil {
		return ""
	}
	return target
}

// AtomicSymlink creates or replaces a symlink at linkPath pointing to target.
// It creates a temporary symlink first and renames it to guarantee atomicity.
func AtomicSymlink(target, linkPath string) error {
	parent := filepath.Dir(linkPath)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return err
	}

	seq := atomic.AddUint64(&symlinkSeq, 1)
	tmpLink := filepath.Join(parent, fmt.Sprintf(".tmp_link_%d_%d_%d", os.Getpid(), time.Now().UnixNano(), seq))
	defer func() {
		_ = os.Remove(tmpLink)
	}()

	if err := makeLink(target, tmpLink); err != nil {
		return err
	}

	return os.Rename(tmpLink, linkPath)
}

// ReplaceDirWithSymlink safely moves existing directory to backup and points original path to target.
func ReplaceDirWithSymlink(originalDir, targetDir, backupDir string) error {
	if _, err := os.Lstat(originalDir); err == nil {
		if err := os.Rename(originalDir, backupDir); err != nil {
			return fmt.Errorf("backup existing dir %s -> %s: %w", originalDir, backupDir, err)
		}
	}

	if err := AtomicSymlink(targetDir, originalDir); err != nil {
		// Attempt to restore backup
		_ = os.Rename(backupDir, originalDir)
		return fmt.Errorf("create symlink %s -> %s: %w", originalDir, targetDir, err)
	}

	return nil
}
