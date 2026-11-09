package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestAtomicSymlinkAndReplace(t *testing.T) {
	tmp := t.TempDir()
	origDir := filepath.Join(tmp, "original")
	targetDir := filepath.Join(tmp, "external_target")
	backupDir := filepath.Join(tmp, "original.bak")

	if err := os.Mkdir(origDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(origDir, "data.txt"), []byte("sample"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(targetDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "external.txt"), []byte("remote"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := ReplaceDirWithSymlink(origDir, targetDir, backupDir); err != nil {
		t.Fatalf("ReplaceDirWithSymlink failed: %v", err)
	}

	if !IsSymlink(origDir) {
		t.Errorf("expected origDir to become a symlink")
	}

	linkTarget := ReadSymlinkTarget(origDir)
	if linkTarget != targetDir {
		t.Errorf("expected symlink target %s, got %s", targetDir, linkTarget)
	}

	// Verify reading through symlink
	readContent, err := os.ReadFile(filepath.Join(origDir, "external.txt"))
	if err != nil || string(readContent) != "remote" {
		t.Errorf("failed reading through symlink: %v", err)
	}

	// Verify backup existence
	backupContent, err := os.ReadFile(filepath.Join(backupDir, "data.txt"))
	if err != nil || string(backupContent) != "sample" {
		t.Errorf("failed reading backup: %v", err)
	}
}

func TestAtomicSymlinkConcurrent(t *testing.T) {
	parent := t.TempDir()
	const workers = 30
	var wg sync.WaitGroup
	errCh := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			targetDir := filepath.Join(parent, fmt.Sprintf("target_%d", id))
			if err := os.Mkdir(targetDir, 0755); err != nil {
				errCh <- err
				return
			}
			linkPath := filepath.Join(parent, fmt.Sprintf("link_%d", id))
			if err := AtomicSymlink(targetDir, linkPath); err != nil {
				errCh <- err
				return
			}
			if target := ReadSymlinkTarget(linkPath); target != targetDir {
				errCh <- fmt.Errorf("symlink target mismatch: got %s, want %s", target, targetDir)
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrent AtomicSymlink error: %v", err)
	}
}
