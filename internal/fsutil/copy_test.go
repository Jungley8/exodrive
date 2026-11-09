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

func TestCopyFile(t *testing.T) {
	tmp := t.TempDir()
	srcFile := filepath.Join(tmp, "source.bin")
	dstFile := filepath.Join(tmp, "sub", "destination.bin")

	content := []byte("binary cache raw content for docker virtual disk testing")
	if err := os.WriteFile(srcFile, content, 0640); err != nil {
		t.Fatal(err)
	}

	var progressCalls int
	var lastCopied, lastTotal int64
	err := CopyFile(srcFile, dstFile, int64(len(content)), func(copied, total int64) {
		progressCalls++
		lastCopied = copied
		lastTotal = total
	})
	if err != nil {
		t.Fatalf("CopyFile failed: %v", err)
	}

	if progressCalls == 0 {
		t.Errorf("expected progress callback to be called")
	}
	if lastCopied != int64(len(content)) || lastTotal != int64(len(content)) {
		t.Errorf("expected copied=%d, total=%d; got copied=%d, total=%d", len(content), len(content), lastCopied, lastTotal)
	}

	dstData, err := os.ReadFile(dstFile)
	if err != nil || string(dstData) != string(content) {
		t.Fatalf("destination file content mismatch: %v", err)
	}

	// Test Directory error
	srcDir := filepath.Join(tmp, "dir")
	if err := os.Mkdir(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := CopyFile(srcDir, filepath.Join(tmp, "out"), 0, nil); err == nil {
		t.Errorf("expected error when source is directory, got nil")
	}

	// Test Nonexistent file error
	if err := CopyFile(filepath.Join(tmp, "nonexistent"), filepath.Join(tmp, "out2"), 0, nil); err == nil {
		t.Errorf("expected error for nonexistent file, got nil")
	}
}
