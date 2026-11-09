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
