//go:build !darwin

package fsutil

import (
	"syscall"
)

// GetDiskUsage returns free bytes, total bytes, and the filesystem name.
func GetDiskUsage(path string) (freeBytes, totalBytes int64, fstype string, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, "", err
	}

	freeBytes = int64(stat.Bavail) * int64(stat.Bsize)
	totalBytes = int64(stat.Blocks) * int64(stat.Bsize)

	return freeBytes, totalBytes, "unix", nil
}
