package plugins

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"exodrive/internal/core"
	"exodrive/internal/fsutil"
)

// findItem returns the item with the given ID, or fails the test.
func findItem(t *testing.T, items []core.CacheItem, id string) core.CacheItem {
	t.Helper()
	for _, item := range items {
		if item.ID == id {
			return item
		}
	}
	t.Fatalf("no item %q in %+v", id, items)
	return core.CacheItem{}
}

// testLinkedMove runs one move and one rollback for a row of a linked provider.
// The folder is removed at the end, so the next row starts from an empty home.
func testLinkedMove(t *testing.T, env *mockEnv, p Provider, id, path, subdir string) {
	t.Helper()
	ctx := context.Background()

	payload := bytes.Repeat([]byte("c"), 2048)
	if err := os.MkdirAll(filepath.Join(path, "entry"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "entry", "data"), payload, 0644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(path) })

	items, err := p.Detect(ctx)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	item := findItem(t, items, id)
	if item.ToolName != p.Name() {
		t.Fatalf("tool name is %q, want %q", item.ToolName, p.Name())
	}
	if item.CurrentPath != path {
		t.Fatalf("current path is %q, want %q", item.CurrentPath, path)
	}
	if item.TargetSubdir != subdir {
		t.Fatalf("target subdir is %q, want %q", item.TargetSubdir, subdir)
	}
	if item.IsMigrated {
		t.Fatal("a folder on this disk must not report a move")
	}
	if item.SizeBytes <= 0 {
		t.Fatalf("size is %d, want the bytes on disk", item.SizeBytes)
	}

	if err := p.PreflightCheck(ctx, item, env.targetRoot); err != nil {
		t.Fatalf("PreflightCheck refused the move: %v", err)
	}
	if err := p.Migrate(ctx, item, env.targetRoot, fsutil.MigrateProgress{}); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	dest := filepath.Join(env.targetRoot, item.TargetSubdir)
	copied, err := os.ReadFile(filepath.Join(dest, "entry", "data"))
	if err != nil {
		t.Fatalf("the copy holds no data: %v", err)
	}
	if !bytes.Equal(copied, payload) {
		t.Fatal("the copy holds different bytes")
	}
	if !fsutil.IsSymlink(path) {
		t.Fatal("the folder is not a link after the move")
	}
	if target := fsutil.ReadSymlinkTarget(path); target != dest {
		t.Fatalf("the link points at %q, want %q", target, dest)
	}

	manifest, err := core.LoadManifest()
	if err != nil {
		t.Fatal(err)
	}
	rec, ok := manifest.Records[item.ID]
	if !ok {
		t.Fatalf("the ledger holds no record for %s", item.ID)
	}
	if rec.TreeDigest == "" {
		t.Fatal("the record holds no digest")
	}
	if rec.FileCount != 1 || rec.TotalBytes != int64(len(payload)) {
		t.Fatalf("the record holds %d files and %d bytes, want 1 and %d", rec.FileCount, rec.TotalBytes, len(payload))
	}

	// The tool reads the old path through the link.
	through, err := os.ReadFile(filepath.Join(path, "entry", "data"))
	if err != nil {
		t.Fatalf("the old path no longer reaches the data: %v", err)
	}
	if !bytes.Equal(through, payload) {
		t.Fatal("the old path returns different bytes")
	}

	if err := p.Rollback(ctx, item); err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}
	if fsutil.IsSymlink(path) {
		t.Fatal("the folder is still a link after the rollback")
	}
	restored, err := os.ReadFile(filepath.Join(path, "entry", "data"))
	if err != nil {
		t.Fatalf("the rollback lost the data: %v", err)
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
}

func TestLifecycle_Xcode(t *testing.T) {
	env := setupMockEnv(t)
	p := NewXcodeProvider()

	cases := []struct{ id, rel, subdir string }{
		{"xcode.derived-data", "Library/Developer/Xcode/DerivedData", "xcode/DerivedData"},
		{"xcode.archives", "Library/Developer/Xcode/Archives", "xcode/Archives"},
		{"xcode.device-support-ios", "Library/Developer/Xcode/iOS DeviceSupport", "xcode/DeviceSupport/iOS"},
		{"xcode.device-support-watchos", "Library/Developer/Xcode/watchOS DeviceSupport", "xcode/DeviceSupport/watchOS"},
		{"xcode.device-support-tvos", "Library/Developer/Xcode/tvOS DeviceSupport", "xcode/DeviceSupport/tvOS"},
		{"xcode.device-support-visionos", "Library/Developer/Xcode/visionOS DeviceSupport", "xcode/DeviceSupport/visionOS"},
		{"xcode.previews", "Library/Developer/Xcode/UserData/Previews", "xcode/Previews"},
		{"xcode.app-cache", "Library/Caches/com.apple.dt.Xcode", "xcode/AppCache"},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			testLinkedMove(t, env, p, tc.id, filepath.Join(env.home, filepath.FromSlash(tc.rel)), filepath.FromSlash(tc.subdir))
		})
	}
}

func TestXcodeSimulatorStaysBlocked(t *testing.T) {
	env := setupMockEnv(t)
	ctx := context.Background()
	p := NewXcodeProvider()

	path := filepath.Join(env.home, "Library", "Developer", "CoreSimulator", "Devices")
	if err := os.MkdirAll(filepath.Join(path, "ABC", "data"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(path) })

	item := findItem(t, mustDetect(t, p), "xcode.simulator-devices")
	if item.BlockedReason == "" {
		t.Fatal("the simulator folder reports no reason")
	}
	for _, want := range []string{"CoreSimulatorService", "external volume", "Keep this folder on this Mac"} {
		if !strings.Contains(item.BlockedReason, want) {
			t.Fatalf("the reason does not say %q: %s", want, item.BlockedReason)
		}
	}
	if item.IsMigrated {
		t.Fatal("a blocked folder must not report a move")
	}

	if err := p.PreflightCheck(ctx, item, env.targetRoot); err == nil {
		t.Fatal("PreflightCheck accepted the simulator folder")
	}
	engine := core.NewEngine(p)
	if err := engine.Preflight(ctx, item, env.targetRoot); err == nil {
		t.Fatal("Engine.Preflight accepted the simulator folder")
	}
	if err := engine.Migrate(ctx, item, env.targetRoot, fsutil.MigrateProgress{}); err == nil {
		t.Fatal("Engine.Migrate moved the simulator folder")
	}
	if _, err := os.Stat(filepath.Join(path, "ABC")); err != nil {
		t.Fatalf("the simulator folder changed: %v", err)
	}
	manifest, err := core.LoadManifest()
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Records) != 0 {
		t.Fatalf("a blocked folder wrote %d records", len(manifest.Records))
	}
}

func TestLifecycle_JVM(t *testing.T) {
	env := setupMockEnv(t)

	t.Run("maven", func(t *testing.T) {
		testLinkedMove(t, env, NewMavenProvider(), "maven.repository",
			filepath.Join(env.home, ".m2", "repository"), filepath.Join("maven", "repository"))
	})
	t.Run("gradle caches", func(t *testing.T) {
		testLinkedMove(t, env, NewGradleProvider(), "gradle.caches",
			filepath.Join(env.home, ".gradle", "caches"), filepath.Join("gradle", "caches"))
	})
	t.Run("gradle wrapper", func(t *testing.T) {
		testLinkedMove(t, env, NewGradleProvider(), "gradle.wrapper",
			filepath.Join(env.home, ".gradle", "wrapper", "dists"), filepath.Join("gradle", "wrapper"))
	})
}

// TestGradleHomeFollowsTheEnvironment keeps the provider on the folder that
// GRADLE_USER_HOME names, not the default.
func TestGradleHomeFollowsTheEnvironment(t *testing.T) {
	setupMockEnv(t)
	root := filepath.Join(t.TempDir(), "gradle-home")
	t.Setenv("GRADLE_USER_HOME", root)

	path := filepath.Join(root, "caches")
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(path) })

	item := findItem(t, mustDetect(t, NewGradleProvider()), "gradle.caches")
	if item.CurrentPath != path {
		t.Fatalf("current path is %q, want %q", item.CurrentPath, path)
	}
}

func TestLifecycle_AppleStores(t *testing.T) {
	env := setupMockEnv(t)

	t.Run("cocoapods cache", func(t *testing.T) {
		testLinkedMove(t, env, NewCocoaPodsProvider(), "cocoapods.cache",
			filepath.Join(env.home, "Library", "Caches", "CocoaPods"),
			filepath.Join("cocoapods", "cache"))
	})
	t.Run("cocoapods repos", func(t *testing.T) {
		testLinkedMove(t, env, NewCocoaPodsProvider(), "cocoapods.repos",
			filepath.Join(env.home, ".cocoapods", "repos"),
			filepath.Join("cocoapods", "repos"))
	})
	t.Run("swiftpm cache", func(t *testing.T) {
		testLinkedMove(t, env, NewSwiftPMProvider(), "swiftpm.cache",
			filepath.Join(env.home, "Library", "Caches", "org.swift.swiftpm"),
			filepath.Join("swiftpm", "cache"))
	})
}

// TestCocoaPodsHomeFollowsTheEnvironment keeps the provider on the folder that
// CP_HOME_DIR names.
func TestCocoaPodsHomeFollowsTheEnvironment(t *testing.T) {
	setupMockEnv(t)
	root := filepath.Join(t.TempDir(), "cp-home")
	t.Setenv("CP_HOME_DIR", root)

	path := filepath.Join(root, "repos")
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(path) })

	item := findItem(t, mustDetect(t, NewCocoaPodsProvider()), "cocoapods.repos")
	if item.CurrentPath != path {
		t.Fatalf("current path is %q, want %q", item.CurrentPath, path)
	}
}

func TestLifecycle_Dart(t *testing.T) {
	env := setupMockEnv(t)

	t.Run("hosted packages", func(t *testing.T) {
		testLinkedMove(t, env, NewDartProvider(), "dart.pub-cache",
			filepath.Join(env.home, ".pub-cache", "hosted"), filepath.Join("dart", "hosted"))
	})
	t.Run("git packages", func(t *testing.T) {
		testLinkedMove(t, env, NewDartProvider(), "dart.pub-git",
			filepath.Join(env.home, ".pub-cache", "git"), filepath.Join("dart", "git"))
	})
}

// TestPubCacheFollowsTheEnvironment keeps the provider on the folder that
// PUB_CACHE names.
func TestPubCacheFollowsTheEnvironment(t *testing.T) {
	setupMockEnv(t)
	root := filepath.Join(t.TempDir(), "pub-cache")
	t.Setenv("PUB_CACHE", root)

	path := filepath.Join(root, "hosted")
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(path) })

	item := findItem(t, mustDetect(t, NewDartProvider()), "dart.pub-cache")
	if item.CurrentPath != path {
		t.Fatalf("current path is %q, want %q", item.CurrentPath, path)
	}
}

func TestLifecycle_Android(t *testing.T) {
	env := setupMockEnv(t)
	testLinkedMove(t, env, NewAndroidProvider(), "android.build-cache",
		filepath.Join(env.home, ".android", "build-cache"), filepath.Join("android", "build-cache"))
}

// TestLifecycle_AndroidAVD moves the emulator images. ANDROID_AVD_HOME names
// the folder, so the test reads the same path on every platform.
func TestLifecycle_AndroidAVD(t *testing.T) {
	env := setupMockEnv(t)
	root := filepath.Join(t.TempDir(), "avd")
	t.Setenv("ANDROID_AVD_HOME", root)
	testLinkedMove(t, env, NewAndroidProvider(), "android.avd", root, filepath.Join("android", "avd"))
}

// TestAndroidRowsKeepTheirOwnLocks keeps one row from waiting on the program of
// another row: an emulator writes the images, and a Gradle build writes the
// cache.
func TestAndroidRowsKeepTheirOwnLocks(t *testing.T) {
	env := setupMockEnv(t)
	ctx := context.Background()
	path := filepath.Join(env.home, ".probe")
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(path) })

	writeMockCLI(t, env.bin, "ps", "echo \"99999 1 /Users/x/Library/Android/sdk/emulator/emulator -avd Pixel\"")
	t.Cleanup(func() { writeMockCLI(t, env.bin, "ps", "exit 0") })

	avd := core.CacheItem{ID: "android.avd", ToolName: "android", Title: "Android Emulator Images", CurrentPath: path}
	err := NewAndroidProvider().PreflightCheck(ctx, avd, env.targetRoot)
	if err == nil || !strings.Contains(err.Error(), "emulator is running") {
		t.Fatalf("PreflightCheck accepted a move while the emulator runs: %v", err)
	}

	cache := core.CacheItem{ID: "android.build-cache", ToolName: "android", Title: "Android Build Cache", CurrentPath: path}
	if err := NewAndroidProvider().PreflightCheck(ctx, cache, env.targetRoot); err != nil {
		t.Fatalf("the build cache waited on the emulator: %v", err)
	}
}

func TestLifecycle_Homebrew(t *testing.T) {
	env := setupMockEnv(t)
	testLinkedMove(t, env, NewHomebrewProvider(), "homebrew.downloads",
		filepath.Join(env.home, "Library", "Caches", "Homebrew", "downloads"),
		filepath.Join("homebrew", "downloads"))
}

// TestHomebrewCacheFollowsTheEnvironment keeps the provider on the folder that
// HOMEBREW_CACHE names.
func TestHomebrewCacheFollowsTheEnvironment(t *testing.T) {
	setupMockEnv(t)
	root := filepath.Join(t.TempDir(), "brew-cache")
	t.Setenv("HOMEBREW_CACHE", root)

	path := filepath.Join(root, "downloads")
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(path) })

	item := findItem(t, mustDetect(t, NewHomebrewProvider()), "homebrew.downloads")
	if item.CurrentPath != path {
		t.Fatalf("current path is %q, want %q", item.CurrentPath, path)
	}
}

// TestLifecycle_Conda follows the package folders that the tool reports. The
// list holds a repeated folder, so the test also proves the provider drops it.
func TestLifecycle_Conda(t *testing.T) {
	env := setupMockEnv(t)
	first := filepath.Join(env.home, "miniconda3", "pkgs")
	second := filepath.Join(env.home, "miniforge3", "pkgs")
	t.Setenv("MOCK_CONDA_A", first)
	t.Setenv("MOCK_CONDA_B", second)
	writeMockCLI(t, env.bin, "conda", `if [ "$1" = "info" ] && [ "$2" = "--json" ]; then
	printf '{"pkgs_dirs": ["%s", "%s", "%s"]}\n' "$MOCK_CONDA_A" "$MOCK_CONDA_B" "$MOCK_CONDA_A"
fi`)
	for _, dir := range []string{first, second} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(filepath.Join(env.home, "miniconda3"))
		_ = os.RemoveAll(filepath.Join(env.home, "miniforge3"))
	})

	items := mustDetect(t, NewCondaProvider())
	if len(items) != 2 {
		t.Fatalf("Detect reports %d package caches, want 2: %+v", len(items), items)
	}
	if items[0].CurrentPath != first || items[1].CurrentPath != second {
		t.Fatalf("Detect reports %q and %q", items[0].CurrentPath, items[1].CurrentPath)
	}
	if items[0].ID == items[1].ID {
		t.Fatalf("both caches share the ID %q", items[0].ID)
	}
	if items[0].TargetSubdir == items[1].TargetSubdir {
		t.Fatalf("both caches share the target %q", items[0].TargetSubdir)
	}
	if again := mustDetect(t, NewCondaProvider()); again[0].ID != items[0].ID || again[1].ID != items[1].ID {
		t.Fatalf("a second scan renames the caches: %q, %q", again[0].ID, again[1].ID)
	}

	t.Run("first", func(t *testing.T) {
		testLinkedMove(t, env, NewCondaProvider(), items[0].ID, first, items[0].TargetSubdir)
	})
	t.Run("second", func(t *testing.T) {
		testLinkedMove(t, env, NewCondaProvider(), items[1].ID, second, items[1].TargetSubdir)
	})
}

// TestCondaStaysQuietWithoutAnAnswer proves that a missing tool adds no card, so
// Exodrive never guesses a prefix.
func TestCondaStaysQuietWithoutAnAnswer(t *testing.T) {
	env := setupMockEnv(t)
	writeMockCLI(t, env.bin, "conda", "exit 1")

	items := mustDetect(t, NewCondaProvider())
	if len(items) != 0 {
		t.Fatalf("Detect reports %d caches without an answer", len(items))
	}
}

// TestCondaReportsABrokenAnswer keeps a bad reply visible instead of silent.
func TestCondaReportsABrokenAnswer(t *testing.T) {
	env := setupMockEnv(t)
	writeMockCLI(t, env.bin, "conda", `echo "not json"`)

	if _, err := NewCondaProvider().Detect(context.Background()); err == nil {
		t.Fatal("Detect accepted a reply it cannot read")
	}
}

// TestNewStoresStayHiddenWithoutAFolder keeps a scan quiet: a folder that does
// not exist must not add a card. Conda has its own tests, because it asks the
// tool for the folder.
func TestNewStoresStayHiddenWithoutAFolder(t *testing.T) {
	setupMockEnv(t)
	ctx := context.Background()

	for _, p := range []Provider{
		NewXcodeProvider(), NewMavenProvider(), NewGradleProvider(),
		NewCocoaPodsProvider(), NewSwiftPMProvider(),
		NewDartProvider(), NewAndroidProvider(), NewHomebrewProvider(),
		NewComposerProvider(), NewNuGetProvider(), NewDevtoolsProvider(), NewRubyProvider(),
	} {
		items, err := p.Detect(ctx)
		if err != nil {
			t.Fatalf("%s: Detect failed: %v", p.Name(), err)
		}
		if len(items) != 0 {
			t.Fatalf("%s reports %d items without a folder", p.Name(), len(items))
		}
	}
}

// TestNewStoresRefuseWhileTheToolRuns covers the process lock of each family.
func TestNewStoresRefuseWhileTheToolRuns(t *testing.T) {
	env := setupMockEnv(t)
	ctx := context.Background()

	cases := []struct {
		name     string
		line     string
		want     string
		provider Provider
	}{
		{"xcode", "/Applications/Xcode.app/Contents/MacOS/Xcode", "Xcode is running", NewXcodeProvider()},
		{"xcodebuild", "/Applications/Xcode.app/Contents/Developer/usr/bin/xcodebuild -project A.xcodeproj", "xcodebuild is running", NewXcodeProvider()},
		{"gradle daemon", "/usr/bin/java -cp /x org.gradle.launcher.daemon.bootstrap.GradleDaemon 8.5", "GradleDaemon is running", NewGradleProvider()},
		{"cocoapods", "/opt/homebrew/bin/pod install", "pod is running", NewCocoaPodsProvider()},
		{"flutter", "/Users/x/flutter/bin/dart run build_runner", "dart is running", NewDartProvider()},
		{"android build", "/opt/homebrew/bin/gradle assemble", "gradle is running", NewAndroidProvider()},
		{"conda", "/opt/miniconda3/bin/conda install numpy", "conda is running", NewCondaProvider()},
		{"homebrew", "/opt/homebrew/bin/brew install jq", "brew is running", NewHomebrewProvider()},
		{"composer", "/usr/local/bin/composer install", "composer is running", NewComposerProvider()},
		{"nuget", "/usr/local/share/dotnet/dotnet build", "dotnet is running", NewNuGetProvider()},
		{"ruby gems", "/usr/local/bin/gem install rails", "gem is running", NewRubyProvider()},
		{"golangci-lint", "/opt/homebrew/bin/golangci-lint run ./...", "golangci-lint is running", NewGolangciLintProvider()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			writeMockCLI(t, env.bin, "ps", "echo \"99999 1 "+tc.line+"\"")
			t.Cleanup(func() { writeMockCLI(t, env.bin, "ps", "exit 0") })

			path := filepath.Join(env.home, ".probe")
			if err := os.MkdirAll(path, 0755); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(path) })

			item := core.CacheItem{ID: "probe.item", ToolName: "probe", Title: "Probe", CurrentPath: path}
			err := tc.provider.PreflightCheck(ctx, item, env.targetRoot)
			if err == nil {
				t.Fatalf("PreflightCheck accepted the move while %s runs", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error is %q, want %q", err, tc.want)
			}
		})
	}
}

// mustDetect runs Detect and fails the test on an error.
func mustDetect(t *testing.T, p Provider) []core.CacheItem {
	t.Helper()
	items, err := p.Detect(context.Background())
	if err != nil {
		t.Fatalf("%s: Detect failed: %v", p.Name(), err)
	}
	return items
}

func TestLifecycle_Composer(t *testing.T) {
	env := setupMockEnv(t)
	testLinkedMove(t, env, NewComposerProvider(), "composer.cache",
		filepath.Join(env.home, "Library", "Caches", "composer"),
		filepath.Join("composer", "cache"))
}

// TestComposerCacheFollowsTheEnvironment keeps the provider on the folder that
// COMPOSER_CACHE_DIR names.
func TestComposerCacheFollowsTheEnvironment(t *testing.T) {
	setupMockEnv(t)
	root := filepath.Join(t.TempDir(), "composer-home")
	t.Setenv("COMPOSER_CACHE_DIR", root)

	path := root
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })

	item := findItem(t, mustDetect(t, NewComposerProvider()), "composer.cache")
	if item.CurrentPath != path {
		t.Fatalf("current path is %q, want %q", item.CurrentPath, path)
	}
}

func TestLifecycle_NuGet(t *testing.T) {
	env := setupMockEnv(t)
	testLinkedMove(t, env, NewNuGetProvider(), "nuget.packages",
		filepath.Join(env.home, ".nuget", "packages"),
		filepath.Join("nuget", "packages"))
}

// TestNuGetFollowsTheEnvironment keeps the provider on the folder that
// NUGET_PACKAGES names.
func TestNuGetFollowsTheEnvironment(t *testing.T) {
	setupMockEnv(t)
	root := filepath.Join(t.TempDir(), "nuget-global")
	t.Setenv("NUGET_PACKAGES", root)

	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })

	item := findItem(t, mustDetect(t, NewNuGetProvider()), "nuget.packages")
	if item.CurrentPath != root {
		t.Fatalf("current path is %q, want %q", item.CurrentPath, root)
	}
}

func TestLifecycle_Devtools(t *testing.T) {
	env := setupMockEnv(t)
	p := NewDevtoolsProvider()

	cases := []struct{ id, folder, subdir string }{
		{"devtools.electron", "electron", filepath.Join("devtools", "electron")},
		{"devtools.node-gyp", "node-gyp", filepath.Join("devtools", "node-gyp")},
		{"devtools.typescript", "typescript", filepath.Join("devtools", "typescript")},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			testLinkedMove(t, env, p, tc.id,
				systemCacheFolder(t, env.home, tc.folder),
				filepath.FromSlash(tc.subdir))
		})
	}
}

// systemCacheFolder names where the operating system keeps a tool's cache
// folder. The test states the rule itself, so a provider that reads a different
// rule on another platform fails here.
func systemCacheFolder(t *testing.T, home, name string) string {
	t.Helper()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Caches", name)
	case "windows":
		appData := os.Getenv("LOCALAPPDATA")
		if appData == "" {
			t.Skip("LOCALAPPDATA is not set")
		}
		return filepath.Join(appData, name)
	default:
		// setupMockEnv sets XDG_CACHE_HOME to <home>/.cache.
		return filepath.Join(home, ".cache", name)
	}
}

// TestLifecycle_Ruby moves the Bundler cache and one user gem cache. The gem
// folder carries the Ruby version, so the provider discovers it on the scan.
func TestLifecycle_Ruby(t *testing.T) {
	env := setupMockEnv(t)
	p := NewRubyProvider()

	t.Run("bundler", func(t *testing.T) {
		testLinkedMove(t, env, p, "ruby.bundler-cache",
			filepath.Join(env.home, ".bundle", "cache"),
			filepath.Join("ruby", "bundler-cache"))
	})
	t.Run("gem cache", func(t *testing.T) {
		gemCache := filepath.Join(env.home, ".gem", "ruby", "3.3.0", "cache")
		item := findItem(t, mustDetectAfterFolder(t, env, p, gemCache), "ruby.gem-cache-"+shortDigest(gemCache))
		testLinkedMove(t, env, p, item.ID, gemCache, item.TargetSubdir)
	})
}

// mustDetectAfterFolder creates the folder that a provider discovers on its own,
// then returns what a scan reports.
func mustDetectAfterFolder(t *testing.T, env *mockEnv, p Provider, path string) []core.CacheItem {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(env.home, ".gem")) })
	return mustDetect(t, p)
}

// TestRubyKeepsOneIDPerGemFolder keeps every gem folder apart, and keeps its
// name across scans.
func TestRubyKeepsOneIDPerGemFolder(t *testing.T) {
	env := setupMockEnv(t)
	first := filepath.Join(env.home, ".gem", "ruby", "3.3.0", "cache")
	second := filepath.Join(env.home, ".gem", "ruby", "3.2.0", "cache")
	for _, dir := range []string{first, second} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(env.home, ".gem")) })

	items := mustDetect(t, NewRubyProvider())
	if len(items) != 2 {
		t.Fatalf("Detect reports %d gem caches, want 2: %+v", len(items), items)
	}
	if items[0].ID == items[1].ID {
		t.Fatalf("both gem caches share the ID %q", items[0].ID)
	}
	if items[0].TargetSubdir == items[1].TargetSubdir {
		t.Fatalf("both gem caches share the target %q", items[0].TargetSubdir)
	}
	if again := mustDetect(t, NewRubyProvider()); again[0].ID != items[0].ID || again[1].ID != items[1].ID {
		t.Fatalf("a second scan renames the caches: %q, %q", again[0].ID, again[1].ID)
	}
}

// TestRubyStaysQuietWithoutAFolder keeps a scan empty on a machine without Ruby.
func TestRubyStaysQuietWithoutAFolder(t *testing.T) {
	setupMockEnv(t)
	if items := mustDetect(t, NewRubyProvider()); len(items) != 0 {
		t.Fatalf("Detect reports %d items without a folder", len(items))
	}
}

// TestLifecycle_BrowserStores moves the store of each browser test tool. The
// environment names the folder, so the test reads the same path on every
// platform the tool supports.
func TestLifecycle_BrowserStores(t *testing.T) {
	env := setupMockEnv(t)

	cases := []struct {
		variable string
		provider Provider
		id       string
		subdir   string
	}{
		{"PLAYWRIGHT_BROWSERS_PATH", NewPlaywrightProvider(), "playwright.browsers", filepath.Join("playwright", "browsers")},
		{"CYPRESS_CACHE_FOLDER", NewCypressProvider(), "cypress.binaries", filepath.Join("cypress", "binaries")},
		{"PUPPETEER_CACHE_DIR", NewPuppeteerProvider(), "puppeteer.browsers", filepath.Join("puppeteer", "browsers")},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), tc.id)
			t.Setenv(tc.variable, root)
			testLinkedMove(t, env, tc.provider, tc.id, root, tc.subdir)
		})
	}
}

// TestPlaywrightKeepsTheHermeticInstallApart reads the value that turns the
// shared folder off. Playwright keeps the browsers in node_modules then, so the
// value 0 must never become a path.
func TestPlaywrightKeepsTheHermeticInstallApart(t *testing.T) {
	env := setupMockEnv(t)
	t.Setenv("PLAYWRIGHT_BROWSERS_PATH", "0")

	for _, item := range mustDetect(t, NewPlaywrightProvider()) {
		if item.CurrentPath == "0" || filepath.Base(filepath.Dir(item.CurrentPath)) == "0" {
			t.Fatalf("the hermetic value became a path: %q", item.CurrentPath)
		}
	}

	// The scan reads the system cache folder instead, which the mock home
	// holds.
	want := systemCacheFolder(t, env.home, "ms-playwright")
	if err := os.MkdirAll(want, 0755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(want) })
	item := findItem(t, mustDetect(t, NewPlaywrightProvider()), "playwright.browsers")
	if item.CurrentPath != want {
		t.Fatalf("current path is %q, want %q", item.CurrentPath, want)
	}
}

// TestBrowserStoresStayQuietWithoutAFolder keeps a scan empty on a machine
// that runs none of these tools.
func TestBrowserStoresStayQuietWithoutAFolder(t *testing.T) {
	setupMockEnv(t)
	for _, p := range []Provider{NewPlaywrightProvider(), NewCypressProvider(), NewPuppeteerProvider()} {
		if items := mustDetect(t, p); len(items) != 0 {
			t.Fatalf("%s reports %d items without a folder", p.Name(), len(items))
		}
	}
}

// TestBrowserStoresFollowTheEnvironment keeps each provider on the folder its
// variable names.
func TestBrowserStoresFollowTheEnvironment(t *testing.T) {
	setupMockEnv(t)
	cases := []struct {
		variable string
		provider Provider
		id       string
	}{
		{"PLAYWRIGHT_BROWSERS_PATH", NewPlaywrightProvider(), "playwright.browsers"},
		{"CYPRESS_CACHE_FOLDER", NewCypressProvider(), "cypress.binaries"},
		{"PUPPETEER_CACHE_DIR", NewPuppeteerProvider(), "puppeteer.browsers"},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "store")
			t.Setenv(tc.variable, root)
			if err := os.MkdirAll(root, 0755); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(root) })

			item := findItem(t, mustDetect(t, tc.provider), tc.id)
			if item.CurrentPath != root {
				t.Fatalf("current path is %q, want %q", item.CurrentPath, root)
			}
		})
	}
}

// TestLifecycle_GoTools moves the caches of the two Go analysis tools.
func TestLifecycle_GoTools(t *testing.T) {
	env := setupMockEnv(t)

	t.Run("golangci-lint", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "golangci-cache")
		t.Setenv("GOLANGCI_LINT_CACHE", root)
		testLinkedMove(t, env, NewGolangciLintProvider(), "golangci-lint.cache",
			root, filepath.Join("go-tools", "golangci-lint"))
	})
	t.Run("gopls", func(t *testing.T) {
		testLinkedMove(t, env, NewGoplsProvider(), "gopls.cache",
			systemCacheFolder(t, env.home, "gopls"),
			filepath.Join("go-tools", "gopls"))
	})
}

// TestGolangciLintFollowsTheEnvironment keeps the provider on the folder that
// GOLANGCI_LINT_CACHE names.
func TestGolangciLintFollowsTheEnvironment(t *testing.T) {
	setupMockEnv(t)
	root := filepath.Join(t.TempDir(), "lint-cache")
	t.Setenv("GOLANGCI_LINT_CACHE", root)
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })

	item := findItem(t, mustDetect(t, NewGolangciLintProvider()), "golangci-lint.cache")
	if item.CurrentPath != root {
		t.Fatalf("current path is %q, want %q", item.CurrentPath, root)
	}
}

// TestGoToolCachesStayQuietWithoutAFolder keeps a scan empty on a machine that
// runs neither tool.
func TestGoToolCachesStayQuietWithoutAFolder(t *testing.T) {
	setupMockEnv(t)
	for _, p := range []Provider{NewGolangciLintProvider(), NewGoplsProvider()} {
		if items := mustDetect(t, p); len(items) != 0 {
			t.Fatalf("%s reports %d items without a folder", p.Name(), len(items))
		}
	}
}

// TestBunIgnoresTheSystemCacheFolder keeps bun on the folder that it fills. The
// install cache holds the packages, and a folder at the system cache path
// belongs to another tool, so a scan claims nothing about it.
func TestBunIgnoresTheSystemCacheFolder(t *testing.T) {
	env := setupMockEnv(t)
	writeMockCLI(t, env.bin, "bun", "exit 1")
	t.Setenv("BUN_INSTALL", filepath.Join(env.home, ".bun"))

	foreign := filepath.Join(env.home, "Library", "Caches", "bun")
	if err := os.MkdirAll(filepath.Join(foreign, "packages"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(foreign) })

	if items := mustDetect(t, NewBunProvider()); len(items) != 0 {
		t.Fatalf("the scan reports a folder that bun does not read: %+v", items)
	}
}
