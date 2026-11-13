//go:build darwin

package fsutil

import (
	"syscall"
)

// filesystemHint names the filesystems the allowlist accepts on this system.
const filesystemHint = "APFS, HFS+, ext2/3/4, btrfs, zfs, xfs, f2fs, or bcachefs"

// GetDiskUsage returns free bytes, total bytes, and the filesystem name.
func GetDiskUsage(path string) (freeBytes, totalBytes int64, fstype string, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, "", err
	}

	freeBytes = int64(stat.Bavail) * int64(stat.Bsize)
	totalBytes = int64(stat.Blocks) * int64(stat.Bsize)

	var nameBytes []byte
	for _, b := range stat.Fstypename {
		if b == 0 {
			break
		}
		nameBytes = append(nameBytes, byte(b))
	}
	fstype = string(nameBytes)

	return freeBytes, totalBytes, fstype, nil
}
