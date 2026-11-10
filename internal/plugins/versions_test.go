package plugins

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"exodrive/internal/core"
)

// emptyToolEnvironment points HOME at a temporary folder and PATH at an empty
// folder, so no tool answers a query.
func emptyToolEnvironment(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	bin := filepath.Join(root, "bin")
	for _, dir := range []string{home, bin} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", bin)
	return home
}

// A tool that does not answer its own query leaves the cache folder unknown. A
// guessed folder would move a cache that the tool does not read, so the item
// stays visible and refuses to move.
func TestProvidersRefuseAGuessedFolder(t *testing.T) {
	home := emptyToolEnvironment(t)

	cases := []struct {
		name    string
		dir     string
		itemID  string
		command string
		detect  func(context.Context) ([]core.CacheItem, error)
	}{
		{
			name: "npm", dir: filepath.Join(home, ".npm"), itemID: "npm.cache",
			command: "npm config get cache",
			detect:  func(ctx context.Context) ([]core.CacheItem, error) { return NewNPMProvider().Detect(ctx) },
		},
		{
			name: "pip", dir: filepath.Join(home, "Library", "Caches", "pip"), itemID: "pip.cache",
			command: "pip cache dir",
			detect:  func(ctx context.Context) ([]core.CacheItem, error) { return NewPipProvider().Detect(ctx) },
		},
		{
			name: "uv", dir: filepath.Join(home, "Library", "Caches", "uv"), itemID: "uv.cache",
			command: "uv cache dir",
			detect:  func(ctx context.Context) ([]core.CacheItem, error) { return NewUVProvider().Detect(ctx) },
		},
		{
			name: "yarn", dir: filepath.Join(home, "Library", "Caches", "Yarn"), itemID: "yarn.cache",
			command: "yarn --version",
			detect:  func(ctx context.Context) ([]core.CacheItem, error) { return NewYarnProvider().Detect(ctx) },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.MkdirAll(tc.dir, 0755); err != nil {
				t.Fatal(err)
			}
			items, err := tc.detect(context.Background())
			if err != nil {
				t.Fatalf("Detect failed: %v", err)
			}
			if len(items) != 1 {
				t.Fatalf("Detect returned %d items, want 1", len(items))
			}
			reason := items[0].BlockedReason
			if !strings.Contains(reason, tc.command) {
				t.Fatalf("blocked reason %q does not name the command %q", reason, tc.command)
			}
			if !strings.Contains(reason, "PATH") {
				t.Fatalf("blocked reason %q does not say what to do", reason)
			}
		})
	}
}

// bun reports its cache only inside a project, so Exodrive derives the folder
// from BUN_INSTALL. A custom install root must win over ~/.bun.
func TestBunDetectFollowsBunInstall(t *testing.T) {
	home := emptyToolEnvironment(t)
	install := filepath.Join(t.TempDir(), "bun-home")
	cacheDir := filepath.Join(install, "install", "cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BUN_INSTALL", install)

	items, err := NewBunProvider().Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("Detect returned %d items, want 1", len(items))
	}
	if items[0].CurrentPath != cacheDir {
		t.Fatalf("detected %s, want %s", items[0].CurrentPath, cacheDir)
	}
	if items[0].BlockedReason != "" {
		t.Fatalf("bun is blocked although BUN_INSTALL names the cache: %s", items[0].BlockedReason)
	}
	_ = home
}

// Docker Desktop 4.14 and earlier name the VM disk Docker.qcow2. Exodrive must
// find it, or those users get no item at all.
func TestDockerDetectFindsTheQcow2Disk(t *testing.T) {
	home := emptyToolEnvironment(t)
	dataDir := filepath.Join(home, "Library", "Containers", "com.docker.docker", "Data", "vms", "0", "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}
	disk := filepath.Join(dataDir, "Docker.qcow2")
	if err := os.WriteFile(disk, []byte("vm"), 0644); err != nil {
		t.Fatal(err)
	}

	items, err := NewDockerProvider().Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	for _, item := range items {
		if item.ID == "docker.desktop" {
			if item.CurrentPath != disk {
				t.Fatalf("detected %s, want %s", item.CurrentPath, disk)
			}
			return
		}
	}
	t.Fatalf("Detect did not report the Docker.qcow2 disk: %+v", items)
}
