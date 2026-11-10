package fsutil

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// IsSafeFilesystem returns true if the filesystem supports POSIX permissions,
// locking, and symlinks reliably. The list is an allowlist: an unknown
// filesystem is refused, so a migration can never silently lose metadata.
func IsSafeFilesystem(fstype string) bool {
	norm := strings.ToLower(strings.TrimSpace(fstype))
	switch norm {
	case "apfs", "hfs", "hfs+", "ext4", "ext3", "ext2", "btrfs", "zfs", "xfs", "f2fs", "bcachefs":
		return true
	default:
		return false
	}
}

// IsSameDevice checks if two paths reside on the same filesystem/device.
func IsSameDevice(path1, path2 string) (bool, error) {
	fi1, err := os.Stat(path1)
	if err != nil {
		return false, err
	}
	fi2, err := os.Stat(path2)
	if err != nil {
		return false, err
	}

	stat1, ok1 := fi1.Sys().(*syscall.Stat_t)
	stat2, ok2 := fi2.Sys().(*syscall.Stat_t)
	if !ok1 || !ok2 {
		return false, nil
	}

	return stat1.Dev == stat2.Dev, nil
}

// DirSize calculates the total size of files under root.
// A symlinked root is resolved, so a migrated cache reports the size of the
// data it now points to. Symlinks inside the tree are not followed, to avoid
// cycles and double counting.
func DirSize(root string) (int64, error) {
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return 0, err
	}

	var total int64
	err = filepath.Walk(resolved, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Skip unreadable files or transient sockets
			return nil
		}
		// Do not follow symlinks
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}
