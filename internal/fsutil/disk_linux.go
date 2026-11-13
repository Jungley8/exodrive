//go:build linux

package fsutil

import (
	"fmt"
	"syscall"
)

// filesystemNames maps a Linux filesystem magic number to its name. Only names
// that the allowlist accepts pass the safety check; every other magic number
// is reported as a hex value, which the allowlist refuses.
var filesystemNames = map[int64]string{
	0xEF53:     "ext4",
	0x9123683E: "btrfs",
	0x2FC12FC1: "zfs",
	0x58465342: "xfs",
	0xF2F52010: "f2fs",
	0xCA451A4E: "bcachefs",
	// Named so a refusal states the real filesystem.
	0x2011BAB0: "exfat",
	0x4D44:     "vfat",
	0x5346544E: "ntfs",
	0x6969:     "nfs",
	0x794C7630: "overlayfs",
	0x65735546: "fuse",
	0xFF534D42: "cifs",
	0xFE534D42: "smb2",
	0x01021994: "tmpfs",
}

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

	fstype, ok := filesystemNames[int64(stat.Type)]
	if !ok {
		fstype = fmt.Sprintf("0x%x", stat.Type)
	}

	return freeBytes, totalBytes, fstype, nil
}
