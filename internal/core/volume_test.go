package core

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// useVolumesRoot points the volume lookup at a temporary folder.
func useVolumesRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	previous := VolumesRoot
	VolumesRoot = root
	t.Cleanup(func() { VolumesRoot = previous })
	return root
}

func TestExternalVolumesListsMountedFolders(t *testing.T) {
	root := useVolumesRoot(t)

	if err := os.Mkdir(filepath.Join(root, "SSD"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "Backup"), 0755); err != nil {
		t.Fatal(err)
	}
	// The boot volume shows as a link. It holds no separate disk to choose.
	if err := os.Symlink("/", filepath.Join(root, "Macintosh HD")); err != nil {
		t.Fatal(err)
	}

	got := ExternalVolumes()
	if len(got) != 2 || got[0] != "Backup" || got[1] != "SSD" {
		t.Fatalf("ExternalVolumes returned %v, want [Backup SSD]", got)
	}
}

func TestCheckTargetReportsADisconnectedVolume(t *testing.T) {
	root := useVolumesRoot(t)
	engine := NewEngine()

	_, err := engine.CheckTarget(filepath.Join(root, "SSD", "caches"))
	if err == nil {
		t.Fatal("expected CheckTarget to refuse a folder on a missing volume")
	}
	message := err.Error()
	if !strings.Contains(message, "is not connected") {
		t.Fatalf("error does not name the disconnected volume: %v", err)
	}
	if !strings.Contains(message, "SSD") {
		t.Fatalf("error does not name the volume: %v", err)
	}

	var targetErr *TargetError
	if !errors.As(err, &targetErr) {
		t.Fatalf("error is not a TargetError: %v", err)
	}
	if targetErr.Volume != "SSD" {
		t.Fatalf("TargetError.Volume is %q, want SSD", targetErr.Volume)
	}
}

func TestCheckTargetReportsAMissingFolder(t *testing.T) {
	root := useVolumesRoot(t)
	engine := NewEngine()

	_, err := engine.CheckTarget(filepath.Join(t.TempDir(), "gone"))
	if err == nil {
		t.Fatal("expected CheckTarget to refuse a folder that does not exist")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("error does not say the folder is missing: %v", err)
	}

	var targetErr *TargetError
	if !errors.As(err, &targetErr) {
		t.Fatalf("error is not a TargetError: %v", err)
	}
	if targetErr.Volume != "" {
		t.Fatalf("TargetError.Volume is %q, want empty", targetErr.Volume)
	}
	_ = root
}

func TestPreflightRefusesADisconnectedVolume(t *testing.T) {
	root := useVolumesRoot(t)
	engine := NewEngine(&MockProvider{name: "mock"})

	item := CacheItem{ID: "fake.cache", ToolName: "mock", Title: "Fake", SizeBytes: 1024}
	err := engine.Preflight(t.Context(), item, filepath.Join(root, "SSD", "caches"))
	if err == nil {
		t.Fatal("expected Preflight to refuse a folder on a missing volume")
	}
	if !strings.Contains(err.Error(), "is not connected") {
		t.Fatalf("error does not name the disconnected volume: %v", err)
	}
}

// TestCheckTargetWarnsOnThisMac keeps the reminder honest: a folder under the
// home folder sits on the volume that holds the caches, so a move there frees
// no space. The check still passes, because the reminder never stops a move.
func TestCheckTargetWarnsOnThisMac(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home folder")
	}
	dir, err := os.MkdirTemp(home, ".exodrive-target-")
	if err != nil {
		t.Skipf("home folder is not writable: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	engine := NewEngine()
	info, err := engine.CheckTarget(dir)
	if err != nil {
		t.Fatalf("CheckTarget refused a folder on this Mac: %v", err)
	}
	if !info.SameVolume {
		t.Fatalf("a folder on this Mac is reported as another volume")
	}

	if !TargetOnThisMac(dir) {
		t.Fatalf("TargetOnThisMac is false for a folder on this Mac")
	}
	if TargetOnThisMac(filepath.Join(dir, "gone")) {
		t.Fatalf("TargetOnThisMac claims a known answer for a missing folder")
	}
}
