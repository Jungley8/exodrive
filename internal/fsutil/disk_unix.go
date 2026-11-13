//go:build unix

package fsutil

import (
	"os"
	"strings"
	"syscall"
)

// sameDevice compares the device number that the system reports for each file.
func sameDevice(path1, path2 string) (bool, error) {
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

// IsOnExternalVolume reports whether path sits under a mount point that is not
// the boot volume. macOS mounts every other volume under /Volumes.
func IsOnExternalVolume(path string) bool {
	return strings.HasPrefix(path, "/Volumes/")
}
