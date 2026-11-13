//go:build windows

package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// Exodrive calls the three Win32 functions itself, so the Windows build needs
// no cgo and no extra module.
var (
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	procGetDiskFreeSpaceEx   = kernel32.NewProc("GetDiskFreeSpaceExW")
	procGetVolumeInformation = kernel32.NewProc("GetVolumeInformationW")
	procGetLogicalDrives     = kernel32.NewProc("GetLogicalDrives")
)

// filesystemHint names the filesystems the allowlist accepts on this system.
const filesystemHint = "NTFS"

// GetDiskUsage returns free bytes, total bytes, and the filesystem name.
func GetDiskUsage(path string) (freeBytes, totalBytes int64, fstype string, err error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, "", err
	}

	var freeForCaller, total, free uint64
	ok, _, callErr := procGetDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(&freeForCaller)),
		uintptr(unsafe.Pointer(&total)),
		uintptr(unsafe.Pointer(&free)),
	)
	if ok == 0 {
		return 0, 0, "", fmt.Errorf("read disk usage of %s: %w", path, callErr)
	}

	name, _, err := volumeInformation(path)
	if err != nil {
		return 0, 0, "", err
	}
	return int64(freeForCaller), int64(total), name, nil
}

// volumeInformation returns the filesystem name and the volume serial number
// of the volume that holds path. The serial number identifies the volume the
// way a device number does on macOS and Linux.
func volumeInformation(path string) (fstype string, serial uint32, err error) {
	root := VolumeRoot(path)
	p, err := syscall.UTF16PtrFromString(root)
	if err != nil {
		return "", 0, err
	}

	var name [64]uint16
	var maxComponent, flags uint32
	ok, _, callErr := procGetVolumeInformation.Call(
		uintptr(unsafe.Pointer(p)),
		0, 0,
		uintptr(unsafe.Pointer(&serial)),
		uintptr(unsafe.Pointer(&maxComponent)),
		uintptr(unsafe.Pointer(&flags)),
		uintptr(unsafe.Pointer(&name[0])),
		uintptr(len(name)),
	)
	if ok == 0 {
		return "", 0, fmt.Errorf("read volume information of %s: %w", root, callErr)
	}
	return syscall.UTF16ToString(name[:]), serial, nil
}

// VolumeRoot returns the drive root that holds path, such as C:\.
func VolumeRoot(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	vol := filepath.VolumeName(abs)
	if vol == "" {
		return abs
	}
	return vol + string(filepath.Separator)
}

// sameDevice compares the volume serial numbers of the two paths.
func sameDevice(path1, path2 string) (bool, error) {
	if _, err := os.Stat(path1); err != nil {
		return false, err
	}
	if _, err := os.Stat(path2); err != nil {
		return false, err
	}
	_, serial1, err := volumeInformation(path1)
	if err != nil {
		return false, err
	}
	_, serial2, err := volumeInformation(path2)
	if err != nil {
		return false, err
	}
	return serial1 == serial2, nil
}

// IsOnExternalVolume reports whether path sits on a drive other than the one
// that holds the home folder.
func IsOnExternalVolume(path string) bool {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return false
	}
	return !strings.EqualFold(VolumeRoot(path), VolumeRoot(home))
}

// DriveRoots lists the drives that are ready to use, such as C:\.
func DriveRoots() []string {
	mask, _, _ := procGetLogicalDrives.Call()
	var roots []string
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		root := string(rune('A'+i)) + ":\\"
		// A drive with no volume information is empty or not ready.
		if _, _, err := volumeInformation(root); err != nil {
			continue
		}
		roots = append(roots, root)
	}
	return roots
}
