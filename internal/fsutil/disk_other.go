//go:build !darwin && !linux && !windows

package fsutil

import "errors"

// GetDiskUsage reports that this system has no implementation. Exodrive refuses
// the migration instead of guessing a number the user cannot check.
func GetDiskUsage(string) (freeBytes, totalBytes int64, fstype string, err error) {
	return 0, 0, "", errors.New("read disk usage: this system has no implementation")
}

// filesystemHint names the filesystems the allowlist accepts on this system.
const filesystemHint = "a filesystem Exodrive supports"

// sameDevice reports that this system gives no device identity, so Exodrive
// cannot tell whether the target sits on the same disk.
func sameDevice(string, string) (bool, error) {
	return false, errors.New("compare devices: this system reports no device")
}

// IsOnExternalVolume always reports false: this system mounts no second volume
// under a known root.
func IsOnExternalVolume(string) bool { return false }
