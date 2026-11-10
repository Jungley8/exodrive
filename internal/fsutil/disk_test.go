package fsutil

import (
	"os"
	"testing"
)

func TestGetDiskUsage(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	free, total, fstype, err := GetDiskUsage(home)
	if err != nil {
		t.Fatalf("GetDiskUsage failed: %v", err)
	}

	if total <= 0 {
		t.Errorf("expected total > 0, got %d", total)
	}
	if free <= 0 {
		t.Errorf("expected free > 0, got %d", free)
	}
	if fstype == "" {
		t.Errorf("expected non-empty fstype")
	}

	t.Logf("Home %s: free=%d GB, total=%d GB, fstype=%s, safe=%v",
		home, free/1024/1024/1024, total/1024/1024/1024, fstype, IsSafeFilesystem(fstype))
}

func TestIsSafeFilesystemFailClosed(t *testing.T) {
	cases := []struct {
		fstype string
		want   bool
	}{
		{"", false},
		{"   ", false},
		{"\t\n", false},
		{"exfat", false},
		{"FAT32", false},
		{"msdos", false},
		{"ntfs", false},
		{"cifs", false},
		{"smbfs", false},
		{"nfs", false},
		{"webdav", false},
		{"overlayfs", false},
		{"unknownfs", false},
		{"apfs", true},
		{"APFS", true},
		{"hfs+", true},
		{"ext4", true},
		{"ext2", true},
		{"btrfs", true},
		{"zfs", true},
		{"xfs", true},
		{"f2fs", true},
		{"unix", false},
	}

	for _, tc := range cases {
		got := IsSafeFilesystem(tc.fstype)
		if got != tc.want {
			t.Errorf("IsSafeFilesystem(%q) = %v; want %v", tc.fstype, got, tc.want)
		}
	}
}
