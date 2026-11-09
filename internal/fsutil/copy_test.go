package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyDir(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "target")

	// Create files including simulated read-only file (like Go mod cache)
	subDir := filepath.Join(src, "sub")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	testFile1 := filepath.Join(src, "file1.txt")
	if err := os.WriteFile(testFile1, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}

	readOnlyFile := filepath.Join(subDir, "readonly.txt")
	if err := os.WriteFile(readOnlyFile, []byte("locked content"), 0444); err != nil {
		t.Fatal(err)
	}

	totalSize, err := DirSize(src)
	if err != nil {
		t.Fatalf("DirSize failed: %v", err)
	}

	var reportedBytes int64
	err = CopyDir(src, dst, totalSize, func(copied, total int64) {
		reportedBytes = copied
	})
	if err != nil {
		t.Fatalf("CopyDir failed: %v", err)
	}

	if reportedBytes != totalSize {
		t.Errorf("expected copied=%d, got %d", totalSize, reportedBytes)
	}

	// Verify content
	copied1, err := os.ReadFile(filepath.Join(dst, "file1.txt"))
	if err != nil || string(copied1) != "hello world" {
		t.Fatalf("failed verifying file1.txt content")
	}

	copied2, err := os.ReadFile(filepath.Join(dst, "sub", "readonly.txt"))
	if err != nil || string(copied2) != "locked content" {
		t.Fatalf("failed verifying readonly.txt content")
	}
}
