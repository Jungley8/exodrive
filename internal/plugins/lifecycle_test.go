package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"exodrive/internal/core"
	"exodrive/internal/fsutil"
)

type mockEnv struct {
	home       string
	bin        string
	targetRoot string
	ledgerPath string
}

func setupMockEnv(t *testing.T) *mockEnv {
	t.Helper()
	tmpDir := t.TempDir()
	home := filepath.Join(tmpDir, "home")
	bin := filepath.Join(tmpDir, "bin")
	target := filepath.Join(tmpDir, "external")
	ledger := filepath.Join(tmpDir, "manifest.json")

	if err := os.MkdirAll(home, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", home)
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	t.Setenv("EXODRIVE_MANIFEST_PATH", ledger)

	// Keep every provider inside the temporary home. A developer's shell may
	// export these, and a test must never migrate a real cache.
	t.Setenv("CARGO_HOME", filepath.Join(home, ".cargo"))
	t.Setenv("GOCACHE", filepath.Join(home, "Library", "Caches", "go-build"))
	t.Setenv("GOMODCACHE", filepath.Join(home, "go", "pkg", "mod"))
	t.Setenv("UV_CACHE_DIR", filepath.Join(home, "Library", "Caches", "uv"))
	t.Setenv("PIP_CACHE_DIR", filepath.Join(home, "Library", "Caches", "pip"))
	t.Setenv("BUN_INSTALL", filepath.Join(home, ".bun"))
	t.Setenv("YARN_CACHE_FOLDER", filepath.Join(home, "Library", "Caches", "Yarn"))
	t.Setenv("ORBSTACK_DATA_DIR", filepath.Join(home, "Library", "Application Support", "OrbStack", "data"))

	// Provide a hermetic process table that holds no tool processes.
	writeMockCLI(t, bin, "ps", "exit 0")

	return &mockEnv{
		home:       home,
		bin:        bin,
		targetRoot: target,
		ledgerPath: ledger,
	}
}

func writeMockCLI(t *testing.T, binDir, name, scriptBody string) {
	t.Helper()
	p := filepath.Join(binDir, name)
	content := fmt.Sprintf("#!/bin/sh\n%s\n", scriptBody)
	if err := os.WriteFile(p, []byte(content), 0755); err != nil {
		t.Fatalf("write mock CLI %s: %v", name, err)
	}
}

func TestLifecycle_NPM(t *testing.T) {
	env := setupMockEnv(t)
	ctx := context.Background()

	cacheDir := filepath.Join(env.home, "npm-cache-data")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "sample.tgz"), []byte("npm-tarball"), 0644); err != nil {
		t.Fatal(err)
	}

	confFile := filepath.Join(env.home, "npm.conf")
	t.Setenv("MOCK_NPM_CACHE", cacheDir)
	t.Setenv("MOCK_NPM_CONF", confFile)

	writeMockCLI(t, env.bin, "npm", `
if [ "$1" = "config" ] && [ "$2" = "get" ] && [ "$3" = "cache" ]; then
	echo "$MOCK_NPM_CACHE"
elif [ "$1" = "config" ] && [ "$2" = "set" ]; then
	echo "$3=$4" > "$MOCK_NPM_CONF"
elif [ "$1" = "config" ] && [ "$2" = "delete" ]; then
	rm -f "$MOCK_NPM_CONF"
fi
`)

	p := NewNPMProvider()
	items, err := p.Detect(ctx)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if len(items) != 1 || items[0].ID != "npm.cache" {
		t.Fatalf("expected npm.cache item, got %+v", items)
	}

	item := items[0]
	if err := p.PreflightCheck(ctx, item, env.targetRoot); err != nil {
		t.Fatalf("PreflightCheck failed: %v", err)
	}

	if err := p.Migrate(ctx, item, env.targetRoot, nil); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Verify target files and config
	targetFile := filepath.Join(env.targetRoot, item.TargetSubdir, "sample.tgz")
	if data, err := os.ReadFile(targetFile); err != nil || string(data) != "npm-tarball" {
		t.Fatalf("target file mismatch: %v", err)
	}

	m, err := core.LoadManifest()
	if err != nil || len(m.Records) != 1 {
		t.Fatalf("manifest record missing: %+v", m)
	}

	if err := p.Rollback(ctx, item); err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	m, _ = core.LoadManifest()
	if len(m.Records) != 0 {
		t.Fatalf("manifest record not removed after rollback: %+v", m)
	}
}

func TestLifecycle_PNPM(t *testing.T) {
	env := setupMockEnv(t)
	ctx := context.Background()

	storeDir := filepath.Join(env.home, "pnpm-store-data")
	if err := os.MkdirAll(storeDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storeDir, "meta.json"), []byte("pnpm-meta"), 0644); err != nil {
		t.Fatal(err)
	}

	confFile := filepath.Join(env.home, "pnpm.conf")
	t.Setenv("MOCK_PNPM_STORE", storeDir)
	t.Setenv("MOCK_PNPM_CONF", confFile)

	writeMockCLI(t, env.bin, "pnpm", `
if [ "$1" = "store" ] && [ "$2" = "path" ]; then
	echo "$MOCK_PNPM_STORE"
elif [ "$1" = "config" ] && [ "$2" = "set" ]; then
	echo "$3=$4" > "$MOCK_PNPM_CONF"
elif [ "$1" = "config" ] && [ "$2" = "delete" ]; then
	rm -f "$MOCK_PNPM_CONF"
fi
`)

	p := NewPNPMProvider()
	items, err := p.Detect(ctx)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if len(items) != 1 || items[0].ID != "pnpm.store" {
		t.Fatalf("expected pnpm.store item, got %+v", items)
	}

	item := items[0]
	if err := p.PreflightCheck(ctx, item, env.targetRoot); err != nil {
		t.Fatalf("PreflightCheck failed: %v", err)
	}

	if err := p.Migrate(ctx, item, env.targetRoot, nil); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	targetFile := filepath.Join(env.targetRoot, item.TargetSubdir, "meta.json")
	if data, err := os.ReadFile(targetFile); err != nil || string(data) != "pnpm-meta" {
		t.Fatalf("target file mismatch: %v", err)
	}

	if err := p.Rollback(ctx, item); err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	m, _ := core.LoadManifest()
	if len(m.Records) != 0 {
		t.Fatalf("manifest record not removed: %+v", m)
	}
}

func TestLifecycle_Yarn(t *testing.T) {
	env := setupMockEnv(t)
	ctx := context.Background()
	// Preflight refuses a migration while the variable pins the cache folder.
	// The mock CLI answers the query instead.
	t.Setenv("YARN_CACHE_FOLDER", "")

	cacheDir := filepath.Join(env.home, "yarn-cache-data")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "yarn.tar"), []byte("yarn-archive"), 0644); err != nil {
		t.Fatal(err)
	}

	confFile := filepath.Join(env.home, "yarn.conf")
	t.Setenv("MOCK_YARN_CACHE", cacheDir)
	t.Setenv("MOCK_YARN_CONF", confFile)

	writeMockCLI(t, env.bin, "yarn", `
if [ "$1" = "cache" ] && [ "$2" = "dir" ]; then
	echo "$MOCK_YARN_CACHE"
elif [ "$1" = "config" ] && [ "$2" = "set" ]; then
	echo "$3=$4" > "$MOCK_YARN_CONF"
fi
`)

	p := NewYarnProvider()
	items, err := p.Detect(ctx)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if len(items) != 1 || items[0].ID != "yarn.cache" {
		t.Fatalf("expected yarn.cache item, got %+v", items)
	}

	item := items[0]
	if err := p.PreflightCheck(ctx, item, env.targetRoot); err != nil {
		t.Fatalf("PreflightCheck failed: %v", err)
	}

	if err := p.Migrate(ctx, item, env.targetRoot, nil); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	targetFile := filepath.Join(env.targetRoot, item.TargetSubdir, "yarn.tar")
	if data, err := os.ReadFile(targetFile); err != nil || string(data) != "yarn-archive" {
		t.Fatalf("target file mismatch: %v", err)
	}

	if err := p.Rollback(ctx, item); err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	m, _ := core.LoadManifest()
	if len(m.Records) != 0 {
		t.Fatalf("manifest record not removed: %+v", m)
	}
}

func TestLifecycle_Cargo_PreservesBin(t *testing.T) {
	env := setupMockEnv(t)
	ctx := context.Background()

	cargoHome := filepath.Join(env.home, ".cargo")
	registryDir := filepath.Join(cargoHome, "registry")
	gitDir := filepath.Join(cargoHome, "git")
	binDir := filepath.Join(cargoHome, "bin")

	_ = os.MkdirAll(registryDir, 0755)
	_ = os.MkdirAll(gitDir, 0755)
	_ = os.MkdirAll(binDir, 0755)

	crateFile := filepath.Join(registryDir, "crate.tar")
	gitFile := filepath.Join(gitDir, "repo.pack")
	binExecutable := filepath.Join(binDir, "cargo-watch")

	_ = os.WriteFile(crateFile, []byte("crate-bytes"), 0644)
	_ = os.WriteFile(gitFile, []byte("git-bytes"), 0644)
	_ = os.WriteFile(binExecutable, []byte("watch-binary"), 0755)

	t.Setenv("CARGO_HOME", cargoHome)

	p := NewCargoProvider()
	items, err := p.Detect(ctx)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items (cargo.registry, cargo.git), got %d", len(items))
	}

	// Verify bin directory is never considered a cache
	for _, it := range items {
		if it.CurrentPath == binDir {
			t.Fatalf("CARGO bin directory was erroneously detected as cache")
		}
	}

	for _, it := range items {
		if err := p.PreflightCheck(ctx, it, env.targetRoot); err != nil {
			t.Fatalf("PreflightCheck failed: %v", err)
		}
		if err := p.Migrate(ctx, it, env.targetRoot, nil); err != nil {
			t.Fatalf("Migrate failed on %s: %v", it.ID, err)
		}
		if !fsutil.IsSymlink(it.CurrentPath) {
			t.Fatalf("expected %s to be replaced with symlink", it.CurrentPath)
		}
	}

	// Assert binary is untouched
	binData, err := os.ReadFile(binExecutable)
	if err != nil || string(binData) != "watch-binary" {
		t.Fatalf("binary file in ~/.cargo/bin was corrupted or removed")
	}

	// Rollback all items
	for _, it := range items {
		if err := p.Rollback(ctx, it); err != nil {
			t.Fatalf("Rollback failed on %s: %v", it.ID, err)
		}
		if fsutil.IsSymlink(it.CurrentPath) {
			t.Fatalf("expected %s to no longer be a symlink after rollback", it.CurrentPath)
		}
	}

	m, _ := core.LoadManifest()
	if len(m.Records) != 0 {
		t.Fatalf("expected 0 manifest records after rollback, got %d", len(m.Records))
	}
}

func TestLifecycle_Go(t *testing.T) {
	env := setupMockEnv(t)
	ctx := context.Background()
	// Preflight refuses a migration while the variables pin the cache paths.
	// The mock CLI answers the queries instead.
	t.Setenv("GOCACHE", "")
	t.Setenv("GOMODCACHE", "")

	gocache := filepath.Join(env.home, "go-build-data")
	gomodcache := filepath.Join(env.home, "go-mod-data")
	goEnvFile := filepath.Join(env.home, "go.env")

	_ = os.MkdirAll(gocache, 0755)
	_ = os.MkdirAll(gomodcache, 0755)
	_ = os.WriteFile(filepath.Join(gocache, "build.a"), []byte("go-build-obj"), 0644)
	_ = os.WriteFile(filepath.Join(gomodcache, "mod.zip"), []byte("go-mod-archive"), 0644)

	t.Setenv("MOCK_GOCACHE", gocache)
	t.Setenv("MOCK_GOMODCACHE", gomodcache)
	t.Setenv("MOCK_GO_ENV", goEnvFile)

	writeMockCLI(t, env.bin, "go", `
if [ "$1" = "env" ] && [ "$2" = "GOCACHE" ]; then
	echo "$MOCK_GOCACHE"
elif [ "$1" = "env" ] && [ "$2" = "GOMODCACHE" ]; then
	echo "$MOCK_GOMODCACHE"
elif [ "$1" = "env" ] && [ "$2" = "-w" ]; then
	echo "$3" >> "$MOCK_GO_ENV"
elif [ "$1" = "env" ] && [ "$2" = "-u" ]; then
	echo "unset $3" >> "$MOCK_GO_ENV"
fi
`)

	p := NewGoProvider()
	items, err := p.Detect(ctx)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 go items, got %d", len(items))
	}

	for _, it := range items {
		if err := p.PreflightCheck(ctx, it, env.targetRoot); err != nil {
			t.Fatalf("PreflightCheck failed: %v", err)
		}
		if err := p.Migrate(ctx, it, env.targetRoot, nil); err != nil {
			t.Fatalf("Migrate failed: %v", err)
		}
	}

	m, _ := core.LoadManifest()
	if len(m.Records) != 2 {
		t.Fatalf("expected 2 records in manifest, got %d", len(m.Records))
	}

	for _, it := range items {
		if err := p.Rollback(ctx, it); err != nil {
			t.Fatalf("Rollback failed: %v", err)
		}
	}

	m, _ = core.LoadManifest()
	if len(m.Records) != 0 {
		t.Fatalf("manifest not empty after rollback")
	}
}

func TestLifecycle_Bun_PreservesBin(t *testing.T) {
	env := setupMockEnv(t)
	ctx := context.Background()

	bunInstall := filepath.Join(env.home, ".bun", "install", "cache")
	bunBin := filepath.Join(env.home, ".bun", "bin")

	_ = os.MkdirAll(bunInstall, 0755)
	_ = os.MkdirAll(bunBin, 0755)

	cacheFile := filepath.Join(bunInstall, "bun-pkg.tar")
	binFile := filepath.Join(bunBin, "bun")

	_ = os.WriteFile(cacheFile, []byte("bun-cache-content"), 0644)
	_ = os.WriteFile(binFile, []byte("bun-bin-content"), 0755)

	// Pin the reported cache so a real bun installation cannot redirect the test.
	t.Setenv("MOCK_BUN_CACHE", bunInstall)
	writeMockCLI(t, env.bin, "bun", `
if [ "$1" = "pm" ] && [ "$2" = "cache" ]; then
	echo "$MOCK_BUN_CACHE"
fi
`)

	p := NewBunProvider()
	items, err := p.Detect(ctx)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if len(items) != 1 || items[0].ID != "bun.cache" {
		t.Fatalf("expected bun.cache item, got %+v", items)
	}

	item := items[0]
	if err := p.PreflightCheck(ctx, item, env.targetRoot); err != nil {
		t.Fatalf("PreflightCheck failed: %v", err)
	}

	if err := p.Migrate(ctx, item, env.targetRoot, nil); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	if !fsutil.IsSymlink(bunInstall) {
		t.Fatalf("expected %s to be a symlink", bunInstall)
	}

	// Verify ~/.bun/bin is untouched
	binData, err := os.ReadFile(binFile)
	if err != nil || string(binData) != "bun-bin-content" {
		t.Fatalf("bun binary in ~/.bun/bin was corrupted or removed")
	}

	if err := p.Rollback(ctx, item); err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	if fsutil.IsSymlink(bunInstall) {
		t.Fatalf("expected %s to not be a symlink after rollback", bunInstall)
	}
}

func TestLifecycle_UV(t *testing.T) {
	env := setupMockEnv(t)
	ctx := context.Background()

	uvCache := filepath.Join(env.home, "Library", "Caches", "uv")
	uvPython := filepath.Join(env.home, ".local", "share", "uv", "python")

	_ = os.MkdirAll(uvCache, 0755)
	_ = os.MkdirAll(uvPython, 0755)

	_ = os.WriteFile(filepath.Join(uvCache, "cache.dat"), []byte("uv-cache-bytes"), 0644)
	_ = os.WriteFile(filepath.Join(uvPython, "python3"), []byte("py3-bytes"), 0755)

	// Without the mock, Detect reads the real uv cache and walks its tree.
	t.Setenv("MOCK_UV_CACHE", uvCache)
	writeMockCLI(t, env.bin, "uv", `
if [ "$1" = "cache" ] && [ "$2" = "dir" ]; then
	echo "$MOCK_UV_CACHE"
fi
`)

	p := NewUVProvider()
	items, err := p.Detect(ctx)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 uv items, got %d", len(items))
	}

	for _, it := range items {
		if err := p.PreflightCheck(ctx, it, env.targetRoot); err != nil {
			t.Fatalf("PreflightCheck failed: %v", err)
		}
		if err := p.Migrate(ctx, it, env.targetRoot, nil); err != nil {
			t.Fatalf("Migrate failed: %v", err)
		}
		if !fsutil.IsSymlink(it.CurrentPath) {
			t.Fatalf("expected %s to be symlink", it.CurrentPath)
		}
	}

	for _, it := range items {
		if err := p.Rollback(ctx, it); err != nil {
			t.Fatalf("Rollback failed: %v", err)
		}
		if fsutil.IsSymlink(it.CurrentPath) {
			t.Fatalf("expected %s to be restored", it.CurrentPath)
		}
	}

	m, _ := core.LoadManifest()
	if len(m.Records) != 0 {
		t.Fatalf("manifest not empty after rollback")
	}
}

func TestLifecycle_Pip(t *testing.T) {
	env := setupMockEnv(t)
	ctx := context.Background()
	// Preflight refuses a migration while the variable pins the cache folder.
	// The mock CLI answers the query instead.
	t.Setenv("PIP_CACHE_DIR", "")

	cacheDir := filepath.Join(env.home, "pip-cache-dir")
	confFile := filepath.Join(env.home, "pip.conf")

	_ = os.MkdirAll(cacheDir, 0755)
	_ = os.WriteFile(filepath.Join(cacheDir, "wheel.whl"), []byte("pip-wheel-bytes"), 0644)

	t.Setenv("MOCK_PIP_CACHE", cacheDir)
	t.Setenv("MOCK_PIP_CONF", confFile)

	writeMockCLI(t, env.bin, "pip", `
if [ "$1" = "cache" ] && [ "$2" = "dir" ]; then
	echo "$MOCK_PIP_CACHE"
elif [ "$1" = "config" ] && [ "$2" = "set" ]; then
	echo "$3=$4" > "$MOCK_PIP_CONF"
elif [ "$1" = "config" ] && [ "$2" = "unset" ]; then
	rm -f "$MOCK_PIP_CONF"
fi
`)

	p := NewPipProvider()
	items, err := p.Detect(ctx)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if len(items) != 1 || items[0].ID != "pip.cache" {
		t.Fatalf("expected pip.cache item, got %+v", items)
	}

	item := items[0]
	if err := p.PreflightCheck(ctx, item, env.targetRoot); err != nil {
		t.Fatalf("PreflightCheck failed: %v", err)
	}

	if err := p.Migrate(ctx, item, env.targetRoot, nil); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	targetFile := filepath.Join(env.targetRoot, item.TargetSubdir, "wheel.whl")
	if data, err := os.ReadFile(targetFile); err != nil || string(data) != "pip-wheel-bytes" {
		t.Fatalf("target file mismatch: %v", err)
	}

	if err := p.Rollback(ctx, item); err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	m, _ := core.LoadManifest()
	if len(m.Records) != 0 {
		t.Fatalf("manifest not empty after rollback")
	}
}

func TestLifecycle_Docker_DockerRawSingleFile(t *testing.T) {
	env := setupMockEnv(t)
	ctx := context.Background()

	dockerRawDir := filepath.Join(env.home, "Library", "Containers", "com.docker.docker", "Data", "vms", "0", "data")
	if err := os.MkdirAll(dockerRawDir, 0755); err != nil {
		t.Fatal(err)
	}
	dockerRawFile := filepath.Join(dockerRawDir, "Docker.raw")

	rawContent := bytes.Repeat([]byte("RAW-VIRTUAL-DISK-IMAGE-DATA-"), 500)
	if err := os.WriteFile(dockerRawFile, rawContent, 0644); err != nil {
		t.Fatal(err)
	}

	p := NewDockerProvider()
	items, err := p.Detect(ctx)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}

	var desktopItem *core.CacheItem
	for _, it := range items {
		if it.ID == "docker.desktop" {
			desktopItem = &it
			break
		}
	}
	if desktopItem == nil {
		t.Fatalf("docker.desktop item not detected")
	}

	if err := p.PreflightCheck(ctx, *desktopItem, env.targetRoot); err != nil {
		t.Fatalf("PreflightCheck failed: %v", err)
	}

	var progressCalled bool
	if err := p.Migrate(ctx, *desktopItem, env.targetRoot, func(copied, total int64) {
		progressCalled = true
	}); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	if !progressCalled {
		t.Errorf("expected progress callback to be called during CopyFile")
	}

	// 1. Verify destination is a FILE (<dest>/Docker.raw), not a directory
	expectedDestFile := filepath.Join(env.targetRoot, desktopItem.TargetSubdir, "Docker.raw")
	fi, err := os.Stat(expectedDestFile)
	if err != nil {
		t.Fatalf("destination Docker.raw does not exist: %v", err)
	}
	if fi.IsDir() {
		t.Fatalf("destination Docker.raw must be a regular file, not a directory")
	}

	destData, err := os.ReadFile(expectedDestFile)
	if err != nil || len(destData) != len(rawContent) {
		t.Fatalf("copied Docker.raw content size mismatch: got %d, want %d", len(destData), len(rawContent))
	}

	// 2. Verify source is now a symlink pointing directly to the file <dest>/Docker.raw
	if !fsutil.IsSymlink(dockerRawFile) {
		t.Fatalf("expected source Docker.raw to be a symlink")
	}
	symlinkTarget := fsutil.ReadSymlinkTarget(dockerRawFile)
	if symlinkTarget != expectedDestFile {
		t.Fatalf("symlink target mismatch: got %q, want %q", symlinkTarget, expectedDestFile)
	}

	// 3. The local copy is gone: a completed migration reclaims the space it moved.
	backupFile := dockerRawFile + ".bak"
	if _, err := os.Lstat(backupFile); !os.IsNotExist(err) {
		t.Fatalf("expected local backup %s to be removed, got %v", backupFile, err)
	}

	// 4. Test Rollback
	if err := p.Rollback(ctx, *desktopItem); err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	if fsutil.IsSymlink(dockerRawFile) {
		t.Fatalf("expected source Docker.raw to no longer be a symlink after rollback")
	}

	restoredData, err := os.ReadFile(dockerRawFile)
	if err != nil || len(restoredData) != len(rawContent) {
		t.Fatalf("restored Docker.raw file corrupted after rollback")
	}

	// The move back removes the external copy.
	if _, err := os.Stat(expectedDestFile); !os.IsNotExist(err) {
		t.Fatalf("expected external copy to be removed after rollback, got %v", err)
	}

	m, _ := core.LoadManifest()
	if _, ok := m.Records[desktopItem.ID]; ok {
		t.Fatalf("manifest record not removed after rollback")
	}
}

func TestLifecycle_Docker_Colima(t *testing.T) {
	env := setupMockEnv(t)
	ctx := context.Background()

	colimaDir := filepath.Join(env.home, ".colima")
	if err := os.MkdirAll(colimaDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(colimaDir, "colima.yaml"), []byte("colima-cfg"), 0644); err != nil {
		t.Fatal(err)
	}

	p := NewDockerProvider()
	items, err := p.Detect(ctx)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}

	var colimaItem *core.CacheItem
	for _, it := range items {
		if it.ID == "docker.colima" {
			colimaItem = &it
			break
		}
	}
	if colimaItem == nil {
		t.Fatalf("docker.colima not detected")
	}

	if err := p.Migrate(ctx, *colimaItem, env.targetRoot, nil); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	if !fsutil.IsSymlink(colimaDir) {
		t.Fatalf("expected %s to be symlink", colimaDir)
	}

	if err := p.Rollback(ctx, *colimaItem); err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	if fsutil.IsSymlink(colimaDir) {
		t.Fatalf("expected %s to be restored directory", colimaDir)
	}
}

func TestLifecycle_Orphan(t *testing.T) {
	env := setupMockEnv(t)
	ctx := context.Background()

	// 1. Orphan Go build cache (>1MB)
	orphanGoBuild := filepath.Join(env.home, "Library", "Caches", "go-build")
	if err := os.MkdirAll(orphanGoBuild, 0755); err != nil {
		t.Fatal(err)
	}
	dummyBig := make([]byte, 1024*1024+500) // 1MB + 500B
	if err := os.WriteFile(filepath.Join(orphanGoBuild, "cache.dat"), dummyBig, 0644); err != nil {
		t.Fatal(err)
	}

	// Active GOCACHE points to another volume
	t.Setenv("GOCACHE", filepath.Join(env.targetRoot, "active-go-cache"))

	// 2. Orphan PNPM cache (>1MB)
	orphanPnpm := filepath.Join(env.home, "Library", "Caches", "pnpm")
	if err := os.MkdirAll(orphanPnpm, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(orphanPnpm, "pnpm.dat"), dummyBig, 0644); err != nil {
		t.Fatal(err)
	}

	p := NewOrphanProvider()
	items, err := p.Detect(ctx)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 orphan items, got %d", len(items))
	}

	for _, it := range items {
		if err := p.PreflightCheck(ctx, it, env.targetRoot); err != nil {
			t.Fatalf("PreflightCheck failed: %v", err)
		}
		if err := p.Migrate(ctx, it, env.targetRoot, nil); err != nil {
			t.Fatalf("Migrate failed: %v", err)
		}

		// Verify local directory was deleted/reclaimed
		if _, err := os.Stat(it.CurrentPath); !os.IsNotExist(err) {
			t.Fatalf("expected original path %s to be deleted after orphan archive", it.CurrentPath)
		}
	}

	m, _ := core.LoadManifest()
	if len(m.Records) != 2 {
		t.Fatalf("expected 2 manifest records for orphans, got %d", len(m.Records))
	}

	// Rollback restores the archived files
	for _, it := range items {
		if err := p.Rollback(ctx, it); err != nil {
			t.Fatalf("Rollback failed: %v", err)
		}
		if _, err := os.Stat(it.CurrentPath); os.IsNotExist(err) {
			t.Fatalf("expected original path %s to be restored after orphan rollback", it.CurrentPath)
		}
	}

	m, _ = core.LoadManifest()
	if len(m.Records) != 0 {
		t.Fatalf("manifest not empty after rollback")
	}
}

func TestPreflightCheck_ProcessRunning(t *testing.T) {
	env := setupMockEnv(t)
	ctx := context.Background()

	// Configure the mock process table to report an active npm process.
	writeMockCLI(t, env.bin, "ps", `
echo "99999 node /usr/local/lib/node_modules/npm/bin/npm-cli.js install"
exit 0
`)

	p := NewNPMProvider()
	item := core.CacheItem{
		ID:          "npm.cache",
		ToolName:    "npm",
		CurrentPath: env.home,
	}

	err := p.PreflightCheck(ctx, item, env.targetRoot)
	if err == nil {
		t.Fatalf("expected PreflightCheck to fail when process is running, got nil")
	}

	expectedSub := "npm is running (PID: 99999): stop active npm processes before migration"
	if err.Error() != expectedSub {
		t.Fatalf("error message mismatch: got %q, want %q", err.Error(), expectedSub)
	}
}

// TestLifecycle_Docker_OrbStackCopiesBeforeRedirect pins the rule that OrbStack
// points at the copy only after the copy holds the data. An engine that starts
// on an empty data_dir loses every image and container.
func TestLifecycle_Docker_OrbStackCopiesBeforeRedirect(t *testing.T) {
	env := setupMockEnv(t)
	ctx := context.Background()

	orbData := filepath.Join(env.home, "Library", "Application Support", "OrbStack", "data")
	if err := os.MkdirAll(filepath.Join(orbData, "docker"), 0755); err != nil {
		t.Fatal(err)
	}
	imageData := []byte("ORBSTACK-VM-DISK")
	if err := os.WriteFile(filepath.Join(orbData, "docker", "data.img"), imageData, 0644); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(env.home, ".orbstack", "config.json")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte(`{"data_dir":"`+orbData+`","keep":"me"}`), 0600); err != nil {
		t.Fatal(err)
	}

	p := NewDockerProvider()
	items, err := p.Detect(ctx)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	var orb *core.CacheItem
	for _, it := range items {
		if it.ID == "docker.orbstack" {
			orb = &it
			break
		}
	}
	if orb == nil {
		t.Fatalf("docker.orbstack not detected: %+v", items)
	}

	if err := p.Migrate(ctx, *orb, env.targetRoot, nil); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	dest := filepath.Join(env.targetRoot, orb.TargetSubdir)
	copied, err := os.ReadFile(filepath.Join(dest, "docker", "data.img"))
	if err != nil || string(copied) != string(imageData) {
		t.Fatalf("destination does not hold the VM data: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("config is not valid JSON: %v", err)
	}
	if got := cfg["data_dir"]; got != dest {
		t.Fatalf("data_dir = %v, want %s", got, dest)
	}
	if got := cfg["keep"]; got != "me" {
		t.Fatalf("Migrate dropped an unrelated config key: %v", cfg)
	}

	if _, err := os.Stat(orbData); !os.IsNotExist(err) {
		t.Fatalf("expected the local copy to be removed, got %v", err)
	}

	// Rollback moves the data back and restores the previous data_dir.
	if err := p.Rollback(ctx, *orb); err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}
	restored, err := os.ReadFile(filepath.Join(orbData, "docker", "data.img"))
	if err != nil || string(restored) != string(imageData) {
		t.Fatalf("rollback did not restore the VM data: %v", err)
	}
	data, _ = os.ReadFile(cfgPath)
	cfg = nil
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if got := cfg["data_dir"]; got != orbData {
		t.Fatalf("data_dir after rollback = %v, want %s", got, orbData)
	}
	if got := cfg["keep"]; got != "me" {
		t.Fatalf("Rollback dropped an unrelated config key: %v", cfg)
	}
}

// TestRollbackFailsClosedWhenDestinationIsMissing pins that a rollback never
// reports success, nor drops the ledger record, when the data is unreachable.
func TestRollbackFailsClosedWhenDestinationIsMissing(t *testing.T) {
	env := setupMockEnv(t)
	ctx := context.Background()

	cacheDir := filepath.Join(env.home, "npm-cache-data")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "a.tgz"), []byte("tarball"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("MOCK_NPM_CACHE", cacheDir)
	t.Setenv("MOCK_NPM_CONF", filepath.Join(env.home, "npm.conf"))
	writeMockCLI(t, env.bin, "npm", `
if [ "$1" = "config" ] && [ "$2" = "get" ] && [ "$3" = "cache" ]; then
	echo "$MOCK_NPM_CACHE"
elif [ "$1" = "config" ] && [ "$2" = "set" ]; then
	echo "$3=$4" > "$MOCK_NPM_CONF"
elif [ "$1" = "config" ] && [ "$2" = "delete" ]; then
	rm -f "$MOCK_NPM_CONF"
fi
`)

	p := NewNPMProvider()
	items, err := p.Detect(ctx)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one npm item, got %+v", items)
	}

	if err := p.Migrate(ctx, items[0], env.targetRoot, nil); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// The external copy disappears: a disk pulled out, or a folder deleted.
	if err := os.RemoveAll(filepath.Join(env.targetRoot, items[0].TargetSubdir)); err != nil {
		t.Fatal(err)
	}

	if err := p.Rollback(ctx, items[0]); err == nil {
		t.Fatalf("expected Rollback to fail when the destination is missing")
	}

	m, err := core.LoadManifest()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Records[items[0].ID]; !ok {
		t.Fatalf("rollback dropped the ledger record although it restored nothing")
	}
}

// TestMigrateRefusesExistingSymlink pins that a cache already linked elsewhere
// is not migrated again onto an empty destination.
func TestMigrateRefusesExistingSymlink(t *testing.T) {
	env := setupMockEnv(t)
	ctx := context.Background()

	realRegistry := filepath.Join(env.home, "real-registry")
	if err := os.MkdirAll(realRegistry, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realRegistry, "crate"), []byte("crate-bytes"), 0644); err != nil {
		t.Fatal(err)
	}

	cargoHome := filepath.Join(env.home, ".cargo")
	if err := os.MkdirAll(cargoHome, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realRegistry, filepath.Join(cargoHome, "registry")); err != nil {
		t.Fatal(err)
	}

	p := NewCargoProvider()
	items, err := p.Detect(ctx)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	var registry *core.CacheItem
	for _, it := range items {
		if it.ID == "cargo.registry" {
			registry = &it
			break
		}
	}
	if registry == nil {
		t.Fatalf("cargo.registry not detected: %+v", items)
	}

	if err := p.Migrate(ctx, *registry, env.targetRoot, nil); err == nil {
		t.Fatalf("expected Migrate to refuse an already linked cache")
	}

	m, err := core.LoadManifest()
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Records) != 0 {
		t.Fatalf("refused migration wrote a ledger record: %+v", m.Records)
	}
}

func TestPreflightRefusesEnvOverride(t *testing.T) {
	env := setupMockEnv(t)
	t.Setenv("npm_config_cache", "/tmp/pinned-npm-cache")

	p := NewNPMProvider()
	item := core.CacheItem{
		ID:          "npm.cache",
		ToolName:    "npm",
		CurrentPath: filepath.Join(env.home, "npm-cache"),
	}
	err := p.PreflightCheck(context.Background(), item, env.targetRoot)
	if err == nil {
		t.Fatal("expected PreflightCheck to refuse while npm_config_cache is set")
	}
	if !strings.Contains(err.Error(), "npm_config_cache") {
		t.Fatalf("error does not name the variable: %v", err)
	}
}

// writeTree creates dir with one file named name holding payload.
func writeTree(t *testing.T, dir, name, payload string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(payload), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreNativeDataReconcilesRestoredCopy(t *testing.T) {
	tmp := t.TempDir()
	original := filepath.Join(tmp, "npm-cache")
	destination := filepath.Join(tmp, "external", "npm-cache")
	writeTree(t, original, "sample.tgz", "payload")
	writeTree(t, destination, "sample.tgz", "payload")

	rec := core.MigrationRecord{
		ID:             "npm.cache",
		ToolName:       "npm",
		OriginalPath:   original,
		DestinationDir: destination,
		RedirectMethod: core.RedirectNativeConfig,
	}
	if err := restoreNativeData(rec); err != nil {
		t.Fatalf("restoreNativeData failed on an already restored copy: %v", err)
	}

	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("destination must be removed after the data is back: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(original, "sample.tgz")); err != nil || string(data) != "payload" {
		t.Fatalf("original data is not intact: %v", err)
	}
}

func TestRestoreNativeDataRefusesDivergentCopy(t *testing.T) {
	tmp := t.TempDir()
	original := filepath.Join(tmp, "npm-cache")
	destination := filepath.Join(tmp, "external", "npm-cache")
	writeTree(t, original, "sample.tgz", "local-write")
	writeTree(t, destination, "sample.tgz", "payload")

	rec := core.MigrationRecord{
		ID:             "npm.cache",
		ToolName:       "npm",
		OriginalPath:   original,
		DestinationDir: destination,
		RedirectMethod: core.RedirectNativeConfig,
	}
	if err := restoreNativeData(rec); err == nil {
		t.Fatal("expected restoreNativeData to refuse a local tree that differs from the copy")
	}

	if _, err := os.Stat(filepath.Join(original, "sample.tgz")); err != nil {
		t.Fatalf("original data was touched: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destination, "sample.tgz")); err != nil {
		t.Fatalf("external copy was removed on a refused rollback: %v", err)
	}
}

func TestRestoreSymlinkReconcilesUnswitchedCopy(t *testing.T) {
	tmp := t.TempDir()
	original := filepath.Join(tmp, "registry")
	destination := filepath.Join(tmp, "external", "registry")
	writeTree(t, original, "crate", "crate-bytes")
	writeTree(t, destination, "crate", "crate-bytes")

	// The switch never ran, so original is still a real directory.
	if err := restoreSymlink(original, destination); err != nil {
		t.Fatalf("restoreSymlink failed on an unswitched copy: %v", err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("destination must be removed: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(original, "crate")); err != nil || string(data) != "crate-bytes" {
		t.Fatalf("original data is not intact: %v", err)
	}
}
