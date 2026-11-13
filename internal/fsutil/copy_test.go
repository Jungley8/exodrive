//go:build unix

package fsutil

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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

	totalSize, err := DirSize(context.Background(), src)
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

// File count and total size pass a copy that damaged a byte or swapped two
// files of the same length. VerifyTree must read the content.
func TestVerifyTreeChecksContent(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a.bin"), []byte("aaaa"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "b.bin"), []byte("bbbb"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("identical trees pass", func(t *testing.T) {
		dst := t.TempDir()
		if err := CopyDir(src, dst, 0, nil); err != nil {
			t.Fatal(err)
		}
		if err := VerifyTree(src, dst); err != nil {
			t.Fatalf("VerifyTree on identical trees failed: %v", err)
		}
	})

	t.Run("a changed byte fails", func(t *testing.T) {
		dst := t.TempDir()
		if err := CopyDir(src, dst, 0, nil); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, "a.bin"), []byte("aaab"), 0644); err != nil {
			t.Fatal(err)
		}
		err := VerifyTree(src, dst)
		if err == nil {
			t.Fatal("expected VerifyTree to report the changed content")
		}
		if !strings.Contains(err.Error(), "a.bin") {
			t.Fatalf("error does not name the file: %v", err)
		}
	})

	t.Run("swapped files fail", func(t *testing.T) {
		dst := t.TempDir()
		if err := CopyDir(src, dst, 0, nil); err != nil {
			t.Fatal(err)
		}
		// The two files keep their names and sizes, so only the content reads
		// tell them apart.
		if err := os.WriteFile(filepath.Join(dst, "a.bin"), []byte("bbbb"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, "b.bin"), []byte("aaaa"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := VerifyTree(src, dst); err == nil {
			t.Fatal("expected VerifyTree to report the swapped files")
		}
	})
}

// A single-file copy goes through CopyFile, so it needs the same content check.
func TestVerifyFileChecksContent(t *testing.T) {
	src := t.TempDir()
	source := filepath.Join(src, "Docker.raw")
	if err := os.WriteFile(source, []byte("vmdata"), 0644); err != nil {
		t.Fatal(err)
	}

	good := t.TempDir()
	copied := filepath.Join(good, "Docker.raw")
	if err := CopyFile(source, copied, 0, nil); err != nil {
		t.Fatal(err)
	}
	if err := VerifyFile(source, copied); err != nil {
		t.Fatalf("VerifyFile on an identical copy failed: %v", err)
	}

	if err := os.WriteFile(copied, []byte("vmDATA"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyFile(source, copied); err == nil {
		t.Fatal("expected VerifyFile to report the changed content")
	}
}

// A socket, a FIFO, or a device file cannot be copied or read. Reading one
// would block, so the copy and the verification must refuse it by name.
func TestSpecialFilesAreRefused(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a.bin"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	pipe := filepath.Join(src, "pipe")
	if err := syscall.Mkfifo(pipe, 0600); err != nil {
		t.Skipf("cannot create a FIFO: %v", err)
	}

	t.Run("copy", func(t *testing.T) {
		err := CopyDir(src, filepath.Join(t.TempDir(), "copy"), 0, nil)
		if err == nil {
			t.Fatal("expected CopyDir to refuse the FIFO")
		}
		if !strings.Contains(err.Error(), "special file") {
			t.Fatalf("error does not name a special file: %v", err)
		}
	})

	t.Run("verification", func(t *testing.T) {
		err := VerifyTree(src, t.TempDir())
		if err == nil {
			t.Fatal("expected VerifyTree to refuse the FIFO")
		}
		if !strings.Contains(err.Error(), "special file") {
			t.Fatalf("error does not name a special file: %v", err)
		}
	})
}

// The window and the command line show the check as its own step, so the check
// must report the bytes it reads.
func TestVerifyTreeProgressReportsEveryByte(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a.bin"), []byte("1234"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "b.bin"), []byte("567"), 0644); err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	if err := CopyDir(src, dst, 0, nil); err != nil {
		t.Fatal(err)
	}

	var last, total int64
	if err := VerifyTreeProgress(src, dst, func(done, bytes int64) {
		last, total = done, bytes
	}); err != nil {
		t.Fatalf("VerifyTreeProgress failed: %v", err)
	}
	if last != 7 || total != 7 {
		t.Fatalf("progress ended at %d of %d bytes, want 7 of 7", last, total)
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

// A destination that holds data would merge two trees. The copy must refuse it
// instead of overwriting files it does not own.
func TestCopyDirRefusesNonEmptyDestination(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}

	dst := t.TempDir()
	if err := os.WriteFile(filepath.Join(dst, "keep.txt"), []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := CopyDir(src, dst, 1, nil); err == nil {
		t.Fatal("expected CopyDir to refuse a destination that holds data")
	}
	if data, err := os.ReadFile(filepath.Join(dst, "keep.txt")); err != nil || string(data) != "keep" {
		t.Fatalf("the existing file changed: %v", err)
	}
}

// Writing through a destination symlink would land outside the target tree.
func TestCopyDirRefusesSymlinkDestination(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}

	real := t.TempDir()
	dst := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, dst); err != nil {
		t.Fatal(err)
	}

	if err := CopyDir(src, dst, 1, nil); err == nil {
		t.Fatal("expected CopyDir to refuse a symbolic link destination")
	}
	if entries, _ := os.ReadDir(real); len(entries) != 0 {
		t.Fatalf("the copy wrote through the link: %v", entries)
	}
}

// A store can hold symbolic links. A copy that drops them still matches on file
// count and size, so VerifyTree must compare the links too.
func TestVerifyTreeChecksSymlinks(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("a.txt", filepath.Join(src, "link")); err != nil {
		t.Fatal(err)
	}

	t.Run("identical trees pass", func(t *testing.T) {
		dst := t.TempDir()
		if err := CopyDir(src, dst, 1, nil); err != nil {
			t.Fatal(err)
		}
		if err := VerifyTree(src, dst); err != nil {
			t.Fatalf("VerifyTree on identical trees failed: %v", err)
		}
	})

	t.Run("a missing link fails", func(t *testing.T) {
		dst := t.TempDir()
		if err := CopyDir(src, dst, 1, nil); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(dst, "link")); err != nil {
			t.Fatal(err)
		}
		if err := VerifyTree(src, dst); err == nil {
			t.Fatal("expected VerifyTree to report the missing link")
		}
	})

	t.Run("a changed target fails", func(t *testing.T) {
		dst := t.TempDir()
		if err := CopyDir(src, dst, 1, nil); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(dst, "link")
		if err := os.Remove(link); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("elsewhere", link); err != nil {
			t.Fatal(err)
		}
		if err := VerifyTree(src, dst); err == nil {
			t.Fatal("expected VerifyTree to report the changed link target")
		}
	})
}
