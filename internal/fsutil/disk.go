package fsutil

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// IsSafeFilesystem returns true if the filesystem supports POSIX permissions,
// locking, and symlinks reliably. The list is an allowlist: an unknown
// filesystem is refused, so a migration can never silently lose metadata.
// NTFS holds junctions and file identity, so it is the one Windows entry.
// exFAT and FAT32 do neither, and stay refused on every system.
func IsSafeFilesystem(fstype string) bool {
	norm := strings.ToLower(strings.TrimSpace(fstype))
	switch norm {
	case "apfs", "hfs", "hfs+", "ext4", "ext3", "ext2", "btrfs", "zfs", "xfs", "f2fs", "bcachefs", "ntfs":
		return true
	default:
		return false
	}
}

// FilesystemHint names the filesystems the allowlist accepts on this system.
func FilesystemHint() string { return filesystemHint }

// IsSameDevice checks if two paths reside on the same filesystem/device.
func IsSameDevice(path1, path2 string) (bool, error) {
	if _, err := os.Stat(path1); err != nil {
		return false, err
	}
	if _, err := os.Stat(path2); err != nil {
		return false, err
	}
	return sameDevice(path1, path2)
}

// DirSize calculates the total size of files under root.
// A symlinked root is resolved, so a migrated cache reports the size of the
// data it now points to. Symlinks inside the tree are not followed, to avoid
// cycles and double counting.
func DirSize(ctx context.Context, root string) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return 0, err
	}

	var total int64
	err = filepath.Walk(resolved, func(path string, info os.FileInfo, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
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
