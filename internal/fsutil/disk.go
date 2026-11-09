package fsutil

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// IsSafeFilesystem returns true if the filesystem supports POSIX permissions,
// locking, and symlinks reliably.
func IsSafeFilesystem(fstype string) bool {
	norm := strings.ToLower(strings.TrimSpace(fstype))
	switch norm {
	case "apfs", "hfs", "hfs+", "ext4", "ext3", "btrfs", "zfs", "xfs":
		return true
	case "msdos", "fat", "fat32", "exfat", "ntfs", "smbfs", "cifs":
		return false
	default:
		// Default to allowing Unix filesystems if unknown, but flag FAT/NTFS
		if strings.Contains(norm, "fat") || strings.Contains(norm, "ntfs") {
			return false
		}
		return true
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
// It skips symlink targets to avoid cycles or counting external drives.
func DirSize(root string) (int64, error) {
	var total int64
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
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
