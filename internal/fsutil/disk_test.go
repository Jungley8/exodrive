package fsutil

import (
	"os"
	"path/filepath"
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
		// NTFS holds junctions and file identity, so it carries a promise.
		{"ntfs", true},
		{"NTFS", true},
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

// TestIsSameDevice covers the two answers that matter: a folder and a child of
// it share a volume, and a folder on a second volume does not.
func TestIsSameDevice(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "cache")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}

	same, err := IsSameDevice(dir, child)
	if err != nil {
		t.Fatalf("IsSameDevice failed: %v", err)
	}
	if !same {
		t.Fatalf("a folder and its child are reported as two volumes")
	}

	if _, err := IsSameDevice(dir, filepath.Join(dir, "missing")); err == nil {
		t.Fatalf("a missing path reported a volume")
	}

	other := secondVolume()
	if other == "" {
		t.Skip("no second volume on this machine")
	}
	same, err = IsSameDevice(dir, other)
	if err != nil {
		t.Fatalf("IsSameDevice across volumes failed: %v", err)
	}
	if same {
		t.Fatalf("%s and %s are reported as one volume", dir, other)
	}
}

// secondVolume returns a mounted folder outside the volume of the home folder,
// or "" when this machine has only one volume.
func secondVolume() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	candidates := []string{"/Volumes", "/dev/shm", "/run", "/mnt"}
	for _, root := range candidates {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			path := filepath.Join(root, entry.Name())
			info, err := os.Stat(path)
			if err != nil || !info.IsDir() {
				continue
			}
			same, err := IsSameDevice(home, path)
			if err == nil && !same {
				return path
			}
		}
	}
	return ""
}
