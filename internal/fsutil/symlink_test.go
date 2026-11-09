package fsutil

import (
	"os"
	"path/filepath"
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
