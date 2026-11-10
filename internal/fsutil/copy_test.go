package fsutil

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestCopyDirPreservesModTime(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "target")

	file := filepath.Join(src, "stamp.txt")
	if err := os.WriteFile(file, []byte("stamp"), 0644); err != nil {
		t.Fatal(err)
	}

	want := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := os.Chtimes(file, want, want); err != nil {
		t.Fatal(err)
	}

	if err := CopyDir(src, dst, 0, nil); err != nil {
		t.Fatalf("CopyDir failed: %v", err)
	}

	info, err := os.Stat(filepath.Join(dst, "stamp.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(want) {
		t.Errorf("modification time not preserved: got %v, want %v", info.ModTime(), want)
	}

	dirInfo, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	srcInfo, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	if !dirInfo.ModTime().Equal(srcInfo.ModTime()) {
		t.Errorf("directory time not preserved: got %v, want %v", dirInfo.ModTime(), srcInfo.ModTime())
	}
}

func TestCopyDirRefusesDestinationInsideSource(t *testing.T) {
	src := t.TempDir()
	inner := filepath.Join(src, "inner")

	if err := CopyDir(src, inner, 0, nil); err == nil {
		t.Fatalf("expected error when destination is inside source, got nil")
	}
	if err := CopyDir(src, src, 0, nil); err == nil {
		t.Fatalf("expected error when destination equals source, got nil")
	}
}

func TestVerifyTree(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "copy")

	if err := os.WriteFile(filepath.Join(src, "a.bin"), []byte("12345"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "nested", "b.bin"), []byte("678"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := CopyDir(src, dst, 0, nil); err != nil {
		t.Fatalf("CopyDir failed: %v", err)
	}
	if err := VerifyTree(src, dst); err != nil {
		t.Fatalf("VerifyTree on identical trees failed: %v", err)
	}

	// A missing file must be reported.
	if err := os.Remove(filepath.Join(dst, "nested", "b.bin")); err != nil {
		t.Fatal(err)
	}
	if err := VerifyTree(src, dst); err == nil {
		t.Fatalf("expected VerifyTree to report a missing file")
	}
}

func TestCopyDirPreservesDirectoryMode(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "target")

	readOnly := filepath.Join(src, "cache-store")
	if err := os.Mkdir(readOnly, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(readOnly, "entry"), []byte("bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(readOnly, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(readOnly, 0755) })

	if err := CopyDir(src, dst, 0, nil); err != nil {
		t.Fatalf("CopyDir failed: %v", err)
	}

	copiedDir := filepath.Join(dst, "cache-store")
	t.Cleanup(func() { _ = os.Chmod(copiedDir, 0755) })

	info, err := os.Stat(copiedDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0555 {
		t.Fatalf("directory mode is %o, want 0555", got)
	}
}

func TestCopyFileRefusesSameFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.bin")
	if err := os.WriteFile(path, []byte("payload"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := CopyFile(path, path, 0, nil); err == nil {
		t.Fatal("expected CopyFile to refuse a copy onto itself")
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "payload" {
		t.Fatalf("source file was damaged: %v", err)
	}
}
