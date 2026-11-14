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
	"time"

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
	t.Setenv("OLLAMA_MODELS", filepath.Join(home, ".ollama", "models"))
	t.Setenv("HF_HOME", filepath.Join(home, ".cache", "huggingface"))
	t.Setenv("GRADLE_USER_HOME", filepath.Join(home, ".gradle"))
	t.Setenv("CP_HOME_DIR", filepath.Join(home, ".cocoapods"))
	t.Setenv("PUB_CACHE", filepath.Join(home, ".pub-cache"))
	t.Setenv("HOMEBREW_CACHE", filepath.Join(home, "Library", "Caches", "Homebrew"))
	t.Setenv("BUNDLE_USER_CACHE", filepath.Join(home, ".bundle", "cache"))
	t.Setenv("COMPOSER_CACHE_DIR", filepath.Join(home, "Library", "Caches", "composer"))
	t.Setenv("NUGET_PACKAGES", filepath.Join(home, ".nuget", "packages"))
	t.Setenv("PLAYWRIGHT_BROWSERS_PATH", filepath.Join(home, "Library", "Caches", "ms-playwright"))
	t.Setenv("CYPRESS_CACHE_FOLDER", filepath.Join(home, "Library", "Caches", "Cypress"))
	t.Setenv("PUPPETEER_CACHE_DIR", filepath.Join(home, ".cache", "puppeteer"))
	t.Setenv("GOLANGCI_LINT_CACHE", filepath.Join(home, "Library", "Caches", "golangci-lint"))
	t.Setenv("ANDROID_AVD_HOME", filepath.Join(home, ".android", "avd"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	// Provide a hermetic process table that holds no tool processes.
	writeMockCLI(t, bin, "ps", "exit 0")
	// conda answers with a folder list, so no test reads the machine's install.
	writeMockCLI(t, bin, "conda", "exit 1")

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

	if err := p.Migrate(ctx, item, env.targetRoot, fsutil.MigrateProgress{}); err != nil {
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
	rec := m.Records["npm.cache"]
	if rec.TreeDigest == "" {
		t.Fatal("the move recorded no digest, so a later check cannot prove the copy")
	}
	if rec.FileCount != 1 || rec.TotalBytes != int64(len("npm-tarball")) {
		t.Fatalf("the record holds %+v, want one file of %d bytes", rec, len("npm-tarball"))
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

	storeRoot := filepath.Join(env.home, "pnpm-store-data")
	storeDir := filepath.Join(storeRoot, "v11")
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
elif [ "$1" = "config" ] && [ "$2" = "get" ]; then
	echo "undefined"
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
	// pnpm appends its version folder to store-dir, so the root is the unit.
	if item.CurrentPath != storeRoot {
		t.Fatalf("expected the store root %s, got %s", storeRoot, item.CurrentPath)
	}
	if err := p.PreflightCheck(ctx, item, env.targetRoot); err != nil {
		t.Fatalf("PreflightCheck failed: %v", err)
	}

	if err := p.Migrate(ctx, item, env.targetRoot, fsutil.MigrateProgress{}); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// The version folder must survive the move, or pnpm starts an empty store.
	targetFile := filepath.Join(env.targetRoot, item.TargetSubdir, "v11", "meta.json")
	if data, err := os.ReadFile(targetFile); err != nil || string(data) != "pnpm-meta" {
		t.Fatalf("target file mismatch: %v", err)
	}

	conf, err := os.ReadFile(confFile)
	if err != nil {
		t.Fatalf("read the pnpm config: %v", err)
	}
	if want := "store-dir=" + filepath.Join(env.targetRoot, item.TargetSubdir); string(conf) != want+"\n" {
		t.Fatalf("store-dir mismatch: got %q, want %q", string(conf), want)
	}

	if err := p.Rollback(ctx, item); err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	if data, err := os.ReadFile(filepath.Join(storeDir, "meta.json")); err != nil || string(data) != "pnpm-meta" {
		t.Fatalf("restored file mismatch: %v", err)
	}

	m, _ := core.LoadManifest()
	if len(m.Records) != 0 {
		t.Fatalf("manifest record not removed: %+v", m)
	}
}

// A recorded move whose store path is gone must fail the scan. An empty store
// would send pnpm to a new location without the packages.
func TestPNPMDetectRefusesMissingStoreAfterMigration(t *testing.T) {
	env := setupMockEnv(t)
	ctx := context.Background()

	missing := filepath.Join(env.home, "pnpm-store-data", "v11")
	t.Setenv("MOCK_PNPM_STORE", missing)
	writeMockCLI(t, env.bin, "pnpm", `
if [ "$1" = "store" ] && [ "$2" = "path" ]; then
	echo "$MOCK_PNPM_STORE"
fi
`)

	if err := core.RecordMigration(core.MigrationRecord{
		ID:             "pnpm.store",
		ToolName:       "pnpm",
		OriginalPath:   filepath.Join(env.home, "pnpm-store-data"),
		DestinationDir: filepath.Join(env.targetRoot, "pnpm-store"),
		RedirectMethod: core.RedirectNativeConfig,
		MigratedAt:     time.Now(),
		SizeBytes:      1024,
	}); err != nil {
		t.Fatal(err)
	}

	_, err := NewPNPMProvider().Detect(ctx)
	if err == nil {
		t.Fatal("expected an error for a recorded store that is missing")
	}
	if !strings.Contains(err.Error(), "rollback pnpm.store") {
		t.Fatalf("expected a rollback hint, got %v", err)
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
if [ "$1" = "--version" ]; then
	echo "1.22.19"
elif [ "$1" = "cache" ] && [ "$2" = "dir" ]; then
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

	if err := p.Migrate(ctx, item, env.targetRoot, fsutil.MigrateProgress{}); err != nil {
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
		if err := p.Migrate(ctx, it, env.targetRoot, fsutil.MigrateProgress{}); err != nil {
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

func TestLifecycle_Antigravity_LeavesSiblingsInPlace(t *testing.T) {
	env := setupMockEnv(t)
	ctx := context.Background()

	appDir := filepath.Join(env.home, ".gemini", "antigravity-cli")
	conversations := filepath.Join(appDir, "conversations")
	brain := filepath.Join(appDir, "brain")
	settingsFile := filepath.Join(appDir, "settings.json")

	if err := os.MkdirAll(conversations, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(brain, "abc-123"), 0755); err != nil {
		t.Fatal(err)
	}

	sessionFile := filepath.Join(conversations, "abc-123.db")
	planFile := filepath.Join(brain, "abc-123", "plan.md")
	_ = os.WriteFile(sessionFile, []byte("session-bytes"), 0644)
	_ = os.WriteFile(planFile, []byte("plan-bytes"), 0644)
	_ = os.WriteFile(settingsFile, []byte(`{"model":"x"}`), 0644)

	p := NewAntigravityProvider()
	items, err := p.Detect(ctx)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items (conversations, brain), got %d", len(items))
	}

	for _, it := range items {
		if it.Category != core.CategorySessionStore {
			t.Fatalf("%s: expected session_store category, got %q", it.ID, it.Category)
		}
		if err := p.PreflightCheck(ctx, it, env.targetRoot); err != nil {
			t.Fatalf("PreflightCheck failed on %s: %v", it.ID, err)
		}
		if err := p.Migrate(ctx, it, env.targetRoot, fsutil.MigrateProgress{}); err != nil {
			t.Fatalf("Migrate failed on %s: %v", it.ID, err)
		}
		if !fsutil.IsSymlink(it.CurrentPath) {
			t.Fatalf("expected %s to be replaced with a symlink", it.CurrentPath)
		}
	}

	// The conversation and its artifact arrive on the target volume.
	if data, err := os.ReadFile(filepath.Join(env.targetRoot, "antigravity", "conversations", "abc-123.db")); err != nil || string(data) != "session-bytes" {
		t.Fatalf("conversation copy is missing or changed")
	}
	if data, err := os.ReadFile(filepath.Join(env.targetRoot, "antigravity", "brain", "abc-123", "plan.md")); err != nil || string(data) != "plan-bytes" {
		t.Fatalf("brain copy is missing or changed")
	}

	// Settings stay where the CLI expects them.
	if data, err := os.ReadFile(settingsFile); err != nil || string(data) != `{"model":"x"}` {
		t.Fatalf("settings.json moved or changed")
	}

	for _, it := range items {
		if err := p.Rollback(ctx, it); err != nil {
			t.Fatalf("Rollback failed on %s: %v", it.ID, err)
		}
		if fsutil.IsSymlink(it.CurrentPath) {
			t.Fatalf("expected %s to be a real folder after rollback", it.CurrentPath)
		}
	}

	if data, err := os.ReadFile(sessionFile); err != nil || string(data) != "session-bytes" {
		t.Fatalf("conversation did not return after rollback")
	}

	m, err := core.LoadManifest()
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}
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
		if err := p.Migrate(ctx, it, env.targetRoot, fsutil.MigrateProgress{}); err != nil {
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

	if err := p.Migrate(ctx, item, env.targetRoot, fsutil.MigrateProgress{}); err != nil {
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
	// Preflight refuses a migration while the variable pins the cache folder.
	t.Setenv("UV_CACHE_DIR", "")

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
		if err := p.Migrate(ctx, it, env.targetRoot, fsutil.MigrateProgress{}); err != nil {
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

	if err := p.Migrate(ctx, item, env.targetRoot, fsutil.MigrateProgress{}); err != nil {
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
	if err := p.Migrate(ctx, *desktopItem, env.targetRoot, fsutil.MigrateProgress{
		Copy: func(copied, total int64) { progressCalled = true },
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

	if err := p.Migrate(ctx, *colimaItem, env.targetRoot, fsutil.MigrateProgress{}); err != nil {
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
		if err := p.Migrate(ctx, it, env.targetRoot, fsutil.MigrateProgress{}); err != nil {
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
echo "99999 1 node /usr/local/lib/node_modules/npm/bin/npm-cli.js install"
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

	if err := p.Migrate(ctx, *orb, env.targetRoot, fsutil.MigrateProgress{}); err != nil {
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

	if err := p.Migrate(ctx, items[0], env.targetRoot, fsutil.MigrateProgress{}); err != nil {
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

	if err := p.Migrate(ctx, *registry, env.targetRoot, fsutil.MigrateProgress{}); err == nil {
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

// uv and bun read their cache path from the environment too. The switch would
// not take effect while the variable is set.
func TestPreflightRefusesUVAndBunEnvOverride(t *testing.T) {
	env := setupMockEnv(t)

	cases := []struct {
		variable string
		provider Provider
		item     core.CacheItem
	}{
		{"UV_CACHE_DIR", NewUVProvider(), core.CacheItem{ID: "uv.cache", ToolName: "uv", CurrentPath: filepath.Join(env.home, "uv")}},
		{"BUN_INSTALL_CACHE_DIR", NewBunProvider(), core.CacheItem{ID: "bun.cache", ToolName: "bun", CurrentPath: filepath.Join(env.home, "bun")}},
	}

	for _, tc := range cases {
		t.Run(tc.variable, func(t *testing.T) {
			t.Setenv(tc.variable, "/tmp/pinned")
			err := tc.provider.PreflightCheck(context.Background(), tc.item, env.targetRoot)
			if err == nil {
				t.Fatalf("expected PreflightCheck to refuse while %s is set", tc.variable)
			}
			if !strings.Contains(err.Error(), tc.variable) {
				t.Fatalf("error does not name %s: %v", tc.variable, err)
			}
		})
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

// Yarn 2 and later read cacheFolder from each project file. A global move does
// not reach them, so the item must stay visible but blocked.
func TestYarnBerryIsBlocked(t *testing.T) {
	env := setupMockEnv(t)
	ctx := context.Background()
	t.Setenv("YARN_CACHE_FOLDER", "")

	cacheDir := filepath.Join(env.home, "berry-cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "pkg.zip"), []byte("berry"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MOCK_YARN_CACHE", cacheDir)

	writeMockCLI(t, env.bin, "yarn", `
if [ "$1" = "--version" ]; then
	echo "4.1.0"
elif [ "$1" = "config" ] && [ "$2" = "get" ]; then
	echo "$MOCK_YARN_CACHE"
fi
`)

	p := NewYarnProvider()
	items, err := p.Detect(ctx)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one item, got %+v", items)
	}

	item := items[0]
	if item.ToolVersion != "4.1.0" {
		t.Fatalf("tool version is %q, want 4.1.0", item.ToolVersion)
	}
	if item.BlockedReason == "" {
		t.Fatal("a Yarn 4 cache must carry a blocked reason")
	}
	if err := p.PreflightCheck(ctx, item, env.targetRoot); err == nil {
		t.Fatal("expected PreflightCheck to refuse a Yarn 4 cache")
	}
	if err := core.NewEngine(p).Preflight(ctx, item, env.targetRoot); err == nil {
		t.Fatal("expected Engine.Preflight to refuse a Yarn 4 cache")
	}
}

// TestLifecycle_ModelStores moves each AI model store to the target volume,
// checks that a link replaces the folder, and moves it back.
func TestLifecycle_ModelStores(t *testing.T) {
	env := setupMockEnv(t)
	ctx := context.Background()

	cases := []struct {
		name string
		path string
	}{
		{"ollama", filepath.Join(env.home, ".ollama", "models")},
		{"huggingface", filepath.Join(env.home, ".cache", "huggingface", "hub")},
		{"lmstudio", filepath.Join(env.home, ".lmstudio", "models")},
	}

	providers := make(map[string]Provider)
	for _, p := range Models() {
		providers[p.Name()] = p
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			blobs := filepath.Join(tc.path, "blobs")
			if err := os.MkdirAll(blobs, 0755); err != nil {
				t.Fatal(err)
			}
			payload := bytes.Repeat([]byte("m"), 4096)
			if err := os.WriteFile(filepath.Join(blobs, "sha256-abc"), payload, 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(tc.path, "index.json"), []byte(`{"models":[]}`), 0644); err != nil {
				t.Fatal(err)
			}

			p := providers[tc.name]
			if p == nil {
				t.Fatalf("%s is not in the registry", tc.name)
			}

			items, err := p.Detect(ctx)
			if err != nil {
				t.Fatalf("Detect failed: %v", err)
			}
			if len(items) != 1 {
				t.Fatalf("expected one item, got %+v", items)
			}
			item := items[0]
			if item.Category != core.CategoryModelStore {
				t.Fatalf("category is %q, want %q", item.Category, core.CategoryModelStore)
			}
			if item.CurrentPath != tc.path {
				t.Fatalf("current path is %q, want %q", item.CurrentPath, tc.path)
			}
			if item.SizeBytes <= 0 {
				t.Fatalf("size is %d, want the bytes on disk", item.SizeBytes)
			}
			if item.IsMigrated {
				t.Fatal("a store on this disk must not report a move")
			}
			if item.TargetSubdir == "" {
				t.Fatal("the item names no folder on the target volume")
			}

			if err := p.PreflightCheck(ctx, item, env.targetRoot); err != nil {
				t.Fatalf("PreflightCheck refused the move: %v", err)
			}
			if err := p.Migrate(ctx, item, env.targetRoot, fsutil.MigrateProgress{}); err != nil {
				t.Fatalf("Migrate failed: %v", err)
			}

			dest := filepath.Join(env.targetRoot, item.TargetSubdir)
			copied, err := os.ReadFile(filepath.Join(dest, "blobs", "sha256-abc"))
			if err != nil {
				t.Fatalf("the copy holds no blob: %v", err)
			}
			if !bytes.Equal(copied, payload) {
				t.Fatal("the copy holds different bytes")
			}
			if !fsutil.IsSymlink(tc.path) {
				t.Fatal("the store is not a link after the move")
			}
			if target := fsutil.ReadSymlinkTarget(tc.path); target != dest {
				t.Fatalf("the link points at %q, want %q", target, dest)
			}

			manifest, err := core.LoadManifest()
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := manifest.Records[item.ID]; !ok {
				t.Fatalf("the ledger holds no record for %s", item.ID)
			}

			// The tool reads the old path through the link, so Exodrive never
			// rewrites the environment variable that names the store.
			through, err := os.ReadFile(filepath.Join(tc.path, "index.json"))
			if err != nil {
				t.Fatalf("the old path no longer reaches the models: %v", err)
			}
			if string(through) != `{"models":[]}` {
				t.Fatalf("the old path returns %q", through)
			}

			if err := p.Rollback(ctx, item); err != nil {
				t.Fatalf("Rollback failed: %v", err)
			}

			if fsutil.IsSymlink(tc.path) {
				t.Fatal("the store is still a link after the rollback")
			}
			restored, err := os.ReadFile(filepath.Join(tc.path, "blobs", "sha256-abc"))
			if err != nil {
				t.Fatalf("the rollback lost the blob: %v", err)
			}
			if !bytes.Equal(restored, payload) {
				t.Fatal("the rollback restored different bytes")
			}
			if _, err := os.Stat(dest); !os.IsNotExist(err) {
				t.Fatalf("the copy on the drive is still there: %v", err)
			}
			manifest, err = core.LoadManifest()
			if err != nil {
				t.Fatal(err)
			}
			if len(manifest.Records) != 0 {
				t.Fatalf("the ledger still holds %d records", len(manifest.Records))
			}
		})
	}
}

// TestModelStoresStayHiddenWithoutAFolder keeps a scan quiet: a tool that is
// not installed must not add a card with a guessed path.
func TestModelStoresStayHiddenWithoutAFolder(t *testing.T) {
	setupMockEnv(t)
	ctx := context.Background()

	for _, p := range Models() {
		items, err := p.Detect(ctx)
		if err != nil {
			t.Fatalf("%s: Detect failed: %v", p.Name(), err)
		}
		if len(items) != 0 {
			t.Fatalf("%s reports %d items without a folder", p.Name(), len(items))
		}
	}
}

func TestLifecycle_Claude_LeavesSettingsAndPluginsInPlace(t *testing.T) {
	env := setupMockEnv(t)
	ctx := context.Background()

	claudeDir := filepath.Join(env.home, ".claude")
	projects := filepath.Join(claudeDir, "projects", "-Users-me-app")
	history := filepath.Join(claudeDir, "file-history", "abc-123")
	settings := filepath.Join(claudeDir, "settings.json")
	plugin := filepath.Join(claudeDir, "plugins", "marketplaces", "x", "plugin.json")

	for _, dir := range []string{projects, history, filepath.Dir(plugin)} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	transcript := filepath.Join(projects, "11111111-2222-3333-4444-555555555555.jsonl")
	_ = os.WriteFile(transcript, []byte(`{"type":"user"}`), 0644)
	_ = os.WriteFile(filepath.Join(history, "file.txt"), []byte("before the edit"), 0644)
	_ = os.WriteFile(settings, []byte(`{"model":"opus"}`), 0644)
	_ = os.WriteFile(plugin, []byte(`{"name":"x"}`), 0644)

	p := NewClaudeProvider()
	items, err := p.Detect(ctx)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items (projects, file-history), got %d", len(items))
	}
	for _, it := range items {
		if it.Category != core.CategorySessionStore {
			t.Fatalf("%s: expected session_store category, got %q", it.ID, it.Category)
		}
		if err := p.Migrate(ctx, it, env.targetRoot, fsutil.MigrateProgress{}); err != nil {
			t.Fatalf("Migrate failed on %s: %v", it.ID, err)
		}
		if !fsutil.IsSymlink(it.CurrentPath) {
			t.Fatalf("expected %s to be replaced with a symlink", it.CurrentPath)
		}
	}

	if data, err := os.ReadFile(filepath.Join(env.targetRoot, "claude", "projects", "-Users-me-app", filepath.Base(transcript))); err != nil || string(data) != `{"type":"user"}` {
		t.Fatalf("conversation copy is missing or changed")
	}
	if data, err := os.ReadFile(filepath.Join(env.targetRoot, "claude", "file-history", "abc-123", "file.txt")); err != nil || string(data) != "before the edit" {
		t.Fatalf("file history copy is missing or changed")
	}
	// The settings, the credentials, and the plugins stay where Claude Code
	// reads them.
	if data, err := os.ReadFile(settings); err != nil || string(data) != `{"model":"opus"}` {
		t.Fatalf("settings.json moved or changed")
	}
	if data, err := os.ReadFile(plugin); err != nil || string(data) != `{"name":"x"}` {
		t.Fatalf("a plugin moved or changed")
	}

	for _, it := range items {
		if err := p.Rollback(ctx, it); err != nil {
			t.Fatalf("Rollback failed on %s: %v", it.ID, err)
		}
		if fsutil.IsSymlink(it.CurrentPath) {
			t.Fatalf("expected %s to be a real folder after rollback", it.CurrentPath)
		}
	}
	if data, err := os.ReadFile(transcript); err != nil || string(data) != `{"type":"user"}` {
		t.Fatalf("conversation did not return after rollback")
	}

	m, err := core.LoadManifest()
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}
	if len(m.Records) != 0 {
		t.Fatalf("expected 0 manifest records after rollback, got %d", len(m.Records))
	}
}

func TestLifecycle_Codex_RollsBackArchivedSessions(t *testing.T) {
	env := setupMockEnv(t)
	ctx := context.Background()

	sessions := filepath.Join(env.home, ".codex", "sessions", "2026", "11", "14")
	archived := filepath.Join(env.home, ".codex", "archived_sessions")
	config := filepath.Join(env.home, ".codex", "config.toml")

	for _, dir := range []string{sessions, archived} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	rollout := filepath.Join(sessions, "rollout-2026-11-14T10-00-00-abc.jsonl")
	old := filepath.Join(archived, "rollout-2026-10-01T10-00-00-def.jsonl")
	_ = os.WriteFile(rollout, []byte(`{"type":"session_meta"}`), 0644)
	_ = os.WriteFile(old, []byte(`{"type":"session_meta","old":true}`), 0644)
	_ = os.WriteFile(config, []byte("model = \"gpt-5\""), 0644)

	p := NewCodexProvider()
	items, err := p.Detect(ctx)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items (sessions, archived_sessions), got %d", len(items))
	}
	for _, it := range items {
		if err := p.Migrate(ctx, it, env.targetRoot, fsutil.MigrateProgress{}); err != nil {
			t.Fatalf("Migrate failed on %s: %v", it.ID, err)
		}
	}
	if data, err := os.ReadFile(filepath.Join(env.targetRoot, "codex", "archived-sessions", filepath.Base(old))); err != nil || string(data) != `{"type":"session_meta","old":true}` {
		t.Fatalf("archived rollout copy is missing or changed")
	}
	if data, err := os.ReadFile(config); err != nil || string(data) != `model = "gpt-5"` {
		t.Fatalf("config.toml moved or changed")
	}

	for _, it := range items {
		if err := p.Rollback(ctx, it); err != nil {
			t.Fatalf("Rollback failed on %s: %v", it.ID, err)
		}
	}
	if data, err := os.ReadFile(rollout); err != nil || string(data) != `{"type":"session_meta"}` {
		t.Fatalf("rollout did not return after rollback")
	}
}

// TestLifecycle_OpenCode_MovesTheDatabaseAsAFile pins the one row that moves a
// file: the database takes the place of the folder, and the rollback gives the
// file back with its bytes.
func TestLifecycle_OpenCode_MovesTheDatabaseAsAFile(t *testing.T) {
	env := setupMockEnv(t)
	t.Setenv("XDG_DATA_HOME", "")
	ctx := context.Background()

	root := filepath.Join(env.home, ".local", "share", "opencode")
	snapshots := filepath.Join(root, "snapshot", "05952fff")
	storage := filepath.Join(root, "storage", "message")
	for _, dir := range []string{snapshots, storage} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	db := filepath.Join(root, "opencode.db")
	_ = os.WriteFile(db, []byte("sqlite-bytes"), 0644)
	_ = os.WriteFile(filepath.Join(snapshots, "object"), []byte("snapshot"), 0644)
	_ = os.WriteFile(filepath.Join(storage, "part.json"), []byte("part"), 0644)
	_ = os.WriteFile(filepath.Join(root, "auth.json"), []byte("{}"), 0600)

	p := NewOpenCodeProvider()
	items, err := p.Detect(ctx)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items (database, snapshot, storage), got %d", len(items))
	}

	var dbItem core.CacheItem
	for _, it := range items {
		if it.Category != core.CategorySessionStore {
			t.Fatalf("%s: expected session_store category, got %q", it.ID, it.Category)
		}
		if it.ID == "opencode.database" {
			dbItem = it
			if it.SizeBytes != int64(len("sqlite-bytes")) {
				t.Fatalf("a file row reports %d bytes, want the size of the file", it.SizeBytes)
			}
		}
		if err := p.Migrate(ctx, it, env.targetRoot, fsutil.MigrateProgress{}); err != nil {
			t.Fatalf("Migrate failed on %s: %v", it.ID, err)
		}
		if !fsutil.IsSymlink(it.CurrentPath) {
			t.Fatalf("expected %s to be replaced with a symlink", it.CurrentPath)
		}
	}

	copied := filepath.Join(env.targetRoot, "opencode", "opencode.db")
	if data, err := os.ReadFile(copied); err != nil || string(data) != "sqlite-bytes" {
		t.Fatalf("database copy is missing or changed")
	}
	if data, err := os.ReadFile(filepath.Join(root, "auth.json")); err != nil || string(data) != "{}" {
		t.Fatalf("auth.json moved or changed")
	}

	if err := p.Rollback(ctx, dbItem); err != nil {
		t.Fatalf("Rollback failed on %s: %v", dbItem.ID, err)
	}
	if fsutil.IsSymlink(db) {
		t.Fatalf("expected %s to be a real file after rollback", db)
	}
	if data, err := os.ReadFile(db); err != nil || string(data) != "sqlite-bytes" {
		t.Fatalf("database did not return with its bytes after rollback")
	}
	if _, err := os.Stat(copied); !os.IsNotExist(err) {
		t.Fatalf("rollback left the copy behind at %s", copied)
	}
}

// TestOpenCode_RefusesAPendingJournal guards the check that stops a move which
// would split a database from pages that it has not written yet.
func TestOpenCode_RefusesAPendingJournal(t *testing.T) {
	env := setupMockEnv(t)
	t.Setenv("XDG_DATA_HOME", "")
	ctx := context.Background()

	root := filepath.Join(env.home, ".local", "share", "opencode")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(root, "opencode.db")
	_ = os.WriteFile(db, []byte("sqlite-bytes"), 0644)

	p := NewOpenCodeProvider()
	items, err := p.Detect(ctx)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if len(items) != 1 || items[0].ID != "opencode.database" {
		t.Fatalf("expected only the database row, got %+v", items)
	}
	if err := p.PreflightCheck(ctx, items[0], env.targetRoot); err != nil {
		t.Fatalf("a database with no journal must pass: %v", err)
	}

	_ = os.WriteFile(db+"-wal", []byte("unwritten page"), 0644)
	err = p.PreflightCheck(ctx, items[0], env.targetRoot)
	if err == nil || !strings.Contains(err.Error(), "-wal") {
		t.Fatalf("a database with a pending journal must fail and name the journal, got %v", err)
	}
}

// TestLifecycle_OpenCode_RollbackKeepsTwoDifferentFiles pins the check that a
// rollback must not delete a copy that differs from the file on this machine.
// A tree check on one file walks nothing, so the same bytes and different bytes
// would both pass: the test holds the two cases apart.
func TestLifecycle_OpenCode_RollbackKeepsTwoDifferentFiles(t *testing.T) {
	env := setupMockEnv(t)
	t.Setenv("XDG_DATA_HOME", "")
	ctx := context.Background()

	root := filepath.Join(env.home, ".local", "share", "opencode")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(root, "opencode.db")
	_ = os.WriteFile(db, []byte("sqlite-bytes"), 0644)

	p := NewOpenCodeProvider()
	items, err := p.Detect(ctx)
	if err != nil || len(items) != 1 {
		t.Fatalf("Detect failed: %v, items %d", err, len(items))
	}
	item := items[0]
	if err := p.Migrate(ctx, item, env.targetRoot, fsutil.MigrateProgress{}); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	copied := filepath.Join(env.targetRoot, "opencode", "opencode.db")

	// The program wrote the file again after the move. The bytes match, so the
	// rollback may remove the copy on the drive.
	if err := os.Remove(db); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(db, []byte("sqlite-bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := p.Rollback(ctx, item); err != nil {
		t.Fatalf("a rollback of two equal files failed: %v", err)
	}
	if _, err := os.Stat(copied); !os.IsNotExist(err) {
		t.Fatalf("rollback left the equal copy behind at %s", copied)
	}

	// The same steps with different bytes: Exodrive must keep both files.
	if err := p.Migrate(ctx, item, env.targetRoot, fsutil.MigrateProgress{}); err != nil {
		t.Fatalf("second Migrate failed: %v", err)
	}
	if err := os.Remove(db); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(db, []byte("newer-bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := p.Rollback(ctx, item); err == nil {
		t.Fatalf("a rollback of two different files must stop")
	}
	if data, err := os.ReadFile(copied); err != nil || string(data) != "sqlite-bytes" {
		t.Fatalf("the differing copy on the drive was removed or changed")
	}
}
