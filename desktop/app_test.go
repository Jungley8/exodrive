package desktop

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oneo-me/nsgo/nsgotest"

	exoCore "exodrive/internal/core"
	"exodrive/internal/fsutil"
)

// fakeProvider stands in for a toolchain so a test can mount the window without
// touching the machine.
type fakeProvider struct {
	name  string
	items []exoCore.CacheItem
	delay time.Duration
	// migrateDelay holds each Migrate call, so a test can act while it runs.
	migrateDelay time.Duration
	// verifyPause holds the check step open, so a test can read its label.
	verifyPause time.Duration
	// hold keeps Migrate open until the test closes it, so a cancel press
	// cannot race the end of the first move.
	hold chan struct{}
}

func (f *fakeProvider) Name() string { return f.name }

func (f *fakeProvider) Detect(ctx context.Context) ([]exoCore.CacheItem, error) {
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	return f.items, nil
}

func (f *fakeProvider) PreflightCheck(ctx context.Context, item exoCore.CacheItem, targetRoot string) error {
	return nil
}

func (f *fakeProvider) Migrate(ctx context.Context, item exoCore.CacheItem, targetRoot string, progress fsutil.MigrateProgress) error {
	if f.migrateDelay > 0 {
		time.Sleep(f.migrateDelay)
	}
	if f.hold != nil {
		<-f.hold
	}
	if progress.Copy != nil {
		progress.Copy(1, 1)
	}
	if f.verifyPause > 0 {
		if progress.Verify != nil {
			progress.Verify(1, 1)
		}
		time.Sleep(f.verifyPause)
	}
	return exoCore.RecordMigration(exoCore.MigrationRecord{
		ID:             item.ID,
		ToolName:       f.name,
		OriginalPath:   item.CurrentPath,
		DestinationDir: targetRoot,
		RedirectMethod: exoCore.RedirectSymlink,
		MigratedAt:     time.Now(),
		SizeBytes:      item.SizeBytes,
	})
}

func (f *fakeProvider) Rollback(ctx context.Context, item exoCore.CacheItem) error {
	return exoCore.RemoveRecord(item.ID)
}

func newFakeEngine(item exoCore.CacheItem) *exoCore.Engine {
	return exoCore.NewEngine(&fakeProvider{name: "fake", items: []exoCore.CacheItem{item}})
}

func newSlowFakeEngine(item exoCore.CacheItem, delay time.Duration) *exoCore.Engine {
	return exoCore.NewEngine(&fakeProvider{name: "fake", items: []exoCore.CacheItem{item}, delay: delay})
}

// useVolumesRoot points the volume lookup at a temporary folder for one test.
func useVolumesRoot(t *testing.T, root string) string {
	t.Helper()
	previous := exoCore.VolumesRoot
	exoCore.VolumesRoot = root
	t.Cleanup(func() { exoCore.VolumesRoot = previous })
	return root
}

// badgeText returns the status badge of a row, or "" when the row is not shown.
func badgeText(app *nsgotest.App, id string) string {
	views := app.Query("id:badge-" + id)
	if len(views) != 1 {
		return ""
	}
	return views[0].Text
}

// TestWindowListsDetectedCaches mounts the real window and checks that a scan
// result reaches the list. The window must repaint after the scan goroutine
// posts its result.
func TestWindowListsDetectedCaches(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))

	item := exoCore.CacheItem{
		ID:          "fake.cache",
		ToolName:    "fake",
		Title:       "Fake Cache",
		Category:    exoCore.CategoryPackageStore,
		CurrentPath: "/tmp/fake-cache",
		SizeBytes:   4096,
		IsActive:    true,
	}

	app := nsgotest.Start(t, NewMain(newFakeEngine(item)), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Fake Cache") })
	app.Settle()

	if !app.Exists("text:Fake Cache") {
		t.Fatalf("the window did not show the scanned cache:\n%s", app.Dump())
	}
	if !app.Exists("text:4.0 KB") {
		t.Fatalf("the window did not show the cache size:\n%s", app.Dump())
	}
	// A migration needs a target folder first.
	if app.Enabled("id:migrate") {
		t.Fatalf("Migrate is enabled without a target folder")
	}
}

// TestWindowMigratesSelectedCaches drives the window as a user does: type a
// target, press Migrate, confirm. The ledger must hold the record afterwards.
func TestWindowMigratesSelectedCaches(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))
	target := t.TempDir()

	item := exoCore.CacheItem{
		ID:          "fake.cache",
		ToolName:    "fake",
		Title:       "Fake Cache",
		Category:    exoCore.CategoryPackageStore,
		CurrentPath: "/tmp/fake-cache",
		SizeBytes:   4096,
		IsActive:    true,
	}

	app := nsgotest.Start(t, NewMain(newFakeEngine(item)), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Fake Cache") })
	app.Settle()

	app.Type("id:target", target)
	app.Settle()
	if !app.Enabled("id:migrate") {
		t.Fatalf("Migrate is disabled after a target folder was entered:\n%s", app.Dump())
	}

	app.Press("id:migrate")
	app.Settle()
	if !app.Exists("id:confirm") {
		t.Fatalf("Migrate did not ask for confirmation:\n%s", app.Dump())
	}
	app.Press("id:confirm-move")

	app.WaitUntil(2*time.Second, func() bool {
		m, err := exoCore.LoadManifest()
		if err != nil {
			return false
		}
		_, ok := m.Records[item.ID]
		return ok
	})

	m, err := exoCore.LoadManifest()
	if err != nil {
		t.Fatal(err)
	}
	rec, ok := m.Records[item.ID]
	if !ok {
		t.Fatalf("the window did not record the migration:\n%s", app.Dump())
	}
	if rec.DestinationDir != target {
		t.Fatalf("logged destination %q, want %q", rec.DestinationDir, target)
	}

	// The rescan marks the item migrated, and nothing stays selected.
	app.WaitUntil(2*time.Second, func() bool { return badgeText(app, item.ID) == "Migrated" })
	if got := badgeText(app, item.ID); got != "Migrated" {
		t.Fatalf("row badge is %q, want Migrated:\n%s", got, app.Dump())
	}
}

// TestWindowRescanRecovers presses Rescan Caches and waits for the window to
// settle. The scan runs in a goroutine and posts its result later, after the
// toolbar has already repainted, so the post must reach the root scope.
func TestWindowRescanRecovers(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))

	item := exoCore.CacheItem{
		ID:          "fake.cache",
		ToolName:    "fake",
		Title:       "Fake Cache",
		Category:    exoCore.CategoryPackageStore,
		CurrentPath: "/tmp/fake-cache",
		SizeBytes:   4096,
		IsActive:    true,
	}

	app := nsgotest.Start(t, NewMain(newSlowFakeEngine(item, 120*time.Millisecond)), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Fake Cache") })
	app.Settle()

	app.Press("id:rescan")
	app.Settle()
	if app.Enabled("id:rescan") {
		t.Fatalf("Rescan stays enabled while a scan runs:\n%s", app.Dump())
	}

	app.WaitUntil(2*time.Second, func() bool { return app.Enabled("id:rescan") })
	if !app.Enabled("id:rescan") {
		t.Fatalf("the scan never finished; the window is stuck:\n%s", app.Dump())
	}
	if !app.Exists("text:Fake Cache") {
		t.Fatalf("the rescan lost the cache list:\n%s", app.Dump())
	}
}

// TestWindowRollsBackMigratedCache drives the rollback path: migrate a cache,
// press Roll back, confirm. The ledger record must be gone and the row must
// show the cache as local again.
func TestWindowRollsBackMigratedCache(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))
	target := t.TempDir()

	item := exoCore.CacheItem{
		ID:          "fake.cache",
		ToolName:    "fake",
		Title:       "Fake Cache",
		Category:    exoCore.CategoryPackageStore,
		CurrentPath: "/tmp/fake-cache",
		SizeBytes:   4096,
		IsActive:    true,
	}

	app := nsgotest.Start(t, NewMain(newFakeEngine(item)), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Fake Cache") })
	app.Settle()

	// No rollback control on a cache that is still local.
	if app.Exists("id:rollback-fake.cache") {
		t.Fatalf("a local cache offers a rollback:\n%s", app.Dump())
	}

	app.Type("id:target", target)
	app.Settle()
	app.Press("id:migrate")
	app.Settle()
	app.Press("id:confirm-move")
	app.WaitUntil(2*time.Second, func() bool { return badgeText(app, item.ID) == "Migrated" })

	if !app.Exists("id:rollback-fake.cache") {
		t.Fatalf("a migrated cache offers no rollback:\n%s", app.Dump())
	}

	app.Press("id:rollback-fake.cache")
	app.Settle()
	if !app.Exists("id:rollback-dialog") {
		t.Fatalf("Roll back did not ask for confirmation:\n%s", app.Dump())
	}

	app.Press("id:rollback-confirm")
	app.WaitUntil(2*time.Second, func() bool {
		m, err := exoCore.LoadManifest()
		if err != nil {
			return false
		}
		_, ok := m.Records[item.ID]
		return !ok
	})

	app.WaitUntil(2*time.Second, func() bool { return badgeText(app, item.ID) == "Local SSD" })
	if got := badgeText(app, item.ID); got != "Local SSD" {
		t.Fatalf("row badge is %q after rollback, want Local SSD:\n%s", got, app.Dump())
	}
}

// TestWindowDeletesAbandonedCache mounts the window, asks to delete an
// abandoned cache, confirms, and checks that the folder is gone.
func TestWindowDeletesAbandonedCache(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))

	orphanDir := filepath.Join(t.TempDir(), "Library", "Caches", "pnpm")
	if err := os.MkdirAll(orphanDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(orphanDir, "cache.dat"), []byte("dead"), 0644); err != nil {
		t.Fatal(err)
	}

	item := exoCore.CacheItem{
		ID:          "orphan.pnpm",
		ToolName:    "pnpm",
		Title:       "Legacy Local PNPM Cache",
		Category:    exoCore.CategoryOrphan,
		CurrentPath: orphanDir,
		SizeBytes:   4,
	}
	local := exoCore.CacheItem{
		ID:          "fake.cache",
		ToolName:    "fake",
		Title:       "Fake Cache",
		Category:    exoCore.CategoryPackageStore,
		CurrentPath: "/tmp/fake-cache",
		SizeBytes:   4096,
		IsActive:    true,
	}

	engine := exoCore.NewEngine(&fakeProvider{name: "fake", items: []exoCore.CacheItem{local, item}})
	app := nsgotest.Start(t, NewMain(engine), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Legacy Local PNPM Cache") })
	app.Settle()

	// A cache that still sits on the internal disk gets no Delete button.
	if app.Exists("id:clean-fake.cache") {
		t.Fatalf("a package cache offers Delete:\n%s", app.Dump())
	}
	if !app.Exists("id:clean-orphan.pnpm") {
		t.Fatalf("an abandoned cache offers no Delete:\n%s", app.Dump())
	}

	app.Press("id:clean-orphan.pnpm")
	app.Settle()
	if !app.Exists("id:clean-dialog") {
		t.Fatalf("Delete did not ask for confirmation:\n%s", app.Dump())
	}

	app.Press("id:clean-confirm")
	app.WaitUntil(2*time.Second, func() bool {
		_, err := os.Stat(orphanDir)
		return os.IsNotExist(err)
	})
	if _, err := os.Stat(orphanDir); !os.IsNotExist(err) {
		t.Fatalf("the abandoned cache still exists: %v", err)
	}
}

// A cache that Exodrive cannot move stays visible. The row explains why, and
// the selection and the main action stay off.
func TestWindowShowsWhyACacheCannotMove(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))

	blocked := exoCore.CacheItem{
		ID:            "yarn.cache",
		ToolName:      "yarn",
		ToolVersion:   "4.1.0",
		Title:         "Yarn Cache",
		Category:      exoCore.CategoryPackageStore,
		CurrentPath:   filepath.Join(t.TempDir(), "berry-cache"),
		SizeBytes:     1024,
		IsActive:      true,
		BlockedReason: "Yarn 4.1.0 reads cacheFolder from each project file.",
	}

	app := nsgotest.Start(t, NewMain(newFakeEngine(blocked)), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Yarn Cache") })
	app.Settle()

	if !app.Exists("id:blocked-yarn.cache") {
		t.Fatalf("the row does not show the reason:\n%s", app.Dump())
	}
	if app.Enabled("id:select-yarn.cache") {
		t.Fatalf("a blocked cache is selectable:\n%s", app.Dump())
	}
	if badgeText(app, blocked.ID) != "Blocked" {
		t.Fatalf("badge is %q, want Blocked", badgeText(app, blocked.ID))
	}

	app.Type("id:target", t.TempDir())
	app.Settle()
	if app.Enabled("id:migrate") {
		t.Fatalf("Migrate is enabled while every cache is blocked:\n%s", app.Dump())
	}
}

// The first run shows three steps, and the window replaces them with the volume
// summary as soon as the user picks a folder.
// The first run must name a missing external volume. The folder picker lists
// only the internal disk, so the guide is the only place that says what to do.
func TestWindowGuidesWhenNoVolumeIsConnected(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))
	useVolumesRoot(t, t.TempDir())

	item := exoCore.CacheItem{
		ID: "fake.cache", ToolName: "fake", Title: "Fake Cache",
		Category: exoCore.CategoryPackageStore, CurrentPath: "/tmp/fake-cache",
		SizeBytes: 4096, IsActive: true,
	}

	app := nsgotest.Start(t, NewMain(newFakeEngine(item)), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("id:guide") })
	app.WaitUntil(2*time.Second, func() bool {
		return strings.Contains(app.Dump(), "No external volume is connected")
	})
}

// A connected volume turns the same line into a ready state.
func TestWindowGuidesWhenAVolumeIsReady(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))
	root := useVolumesRoot(t, t.TempDir())
	if err := os.Mkdir(filepath.Join(root, "SSD"), 0755); err != nil {
		t.Fatal(err)
	}

	item := exoCore.CacheItem{
		ID: "fake.cache", ToolName: "fake", Title: "Fake Cache",
		Category: exoCore.CategoryPackageStore, CurrentPath: "/tmp/fake-cache",
		SizeBytes: 4096, IsActive: true,
	}

	app := nsgotest.Start(t, NewMain(newFakeEngine(item)), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("id:guide") })
	app.WaitUntil(2*time.Second, func() bool {
		return strings.Contains(app.Dump(), "External volume ready: SSD")
	})
}

// The check steps through every byte, so the window must name it. A footer
// that still says "Moving ... 100%" looks frozen.
func TestWindowShowsTheCheckStep(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))

	item := exoCore.CacheItem{
		ID: "fake.cache", ToolName: "fake", Title: "Fake Cache",
		Category: exoCore.CategoryPackageStore, CurrentPath: "/tmp/fake-cache",
		SizeBytes: 4096, IsActive: true,
	}

	engine := exoCore.NewEngine(&fakeProvider{
		name: "fake", items: []exoCore.CacheItem{item},
		verifyPause: 800 * time.Millisecond,
	})

	app := nsgotest.Start(t, NewMain(engine), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Fake Cache") })

	app.Type("id:target", t.TempDir())
	app.Settle()
	app.Press("id:migrate")
	app.Settle()
	app.Press("id:confirm-move")

	app.WaitUntil(2*time.Second, func() bool {
		return strings.Contains(app.Dump(), "Checking 1 of 1")
	})
}

func TestWindowGuidesTheFirstRun(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))

	item := exoCore.CacheItem{
		ID: "fake.cache", ToolName: "fake", Title: "Fake Cache",
		Category: exoCore.CategoryPackageStore, CurrentPath: "/tmp/fake-cache",
		SizeBytes: 4096, IsActive: true,
	}

	app := nsgotest.Start(t, NewMain(newFakeEngine(item)), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Fake Cache") })
	app.Settle()

	if !app.Exists("id:guide") {
		t.Fatalf("the first run shows no steps:\n%s", app.Dump())
	}

	app.Type("id:target", t.TempDir())
	app.WaitUntil(2*time.Second, func() bool { return !app.Exists("id:guide") })

	app.WaitUntil(2*time.Second, func() bool { return app.Exists("id:target-info") })
	views := app.Query("id:target-info")
	if len(views) != 1 || !strings.HasPrefix(views[0].Text, "Ready.") {
		t.Fatalf("target summary is %+v, want a Ready line", views)
	}
}

// A long run must stop between moves, and it must say how far it got.
func TestWindowStopsAfterTheCancelledMove(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))

	first := exoCore.CacheItem{
		ID: "fake.one", ToolName: "fake", Title: "First Cache",
		Category: exoCore.CategoryPackageStore, CurrentPath: "/tmp/first",
		SizeBytes: 1024, IsActive: true,
	}
	second := exoCore.CacheItem{
		ID: "fake.two", ToolName: "fake", Title: "Second Cache",
		Category: exoCore.CategoryPackageStore, CurrentPath: "/tmp/second",
		SizeBytes: 2048, IsActive: true,
	}

	hold := make(chan struct{})
	engine := exoCore.NewEngine(&fakeProvider{
		name: "fake", items: []exoCore.CacheItem{first, second},
		hold: hold,
	})

	app := nsgotest.Start(t, NewMain(engine), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:First Cache") })

	app.Type("id:target", t.TempDir())
	app.Settle()
	app.Press("id:migrate")
	app.Settle()
	app.Press("id:confirm-move")

	app.WaitUntil(2*time.Second, func() bool { return app.Exists("id:cancel-move") })
	app.Press("id:cancel-move")
	// The footer must answer the press before the current cache ends.
	app.WaitUntil(2*time.Second, func() bool {
		return strings.Contains(app.Dump(), "Stopping after cache 1 of 2")
	})
	// Let the first move finish. The loop must see the cancel before it starts
	// the second cache.
	close(hold)

	app.WaitUntil(3*time.Second, func() bool { return strings.Contains(app.Dump(), "Stopped after 1 of 2 moves") })
	// The first cache moved; the second stayed local.
	if got := badgeText(app, first.ID); got != "Migrated" {
		t.Fatalf("the first cache shows %q, want Migrated", got)
	}
	if got := badgeText(app, second.ID); got == "Migrated" {
		t.Fatalf("the second cache moved after the cancel")
	}
}

// A row says how Exodrive points the tool at the copy, and the confirmation
// counts both kinds before the move starts.
func TestWindowShowsTheSwitchMethod(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))

	configItem := exoCore.CacheItem{
		ID: "npm.cache", ToolName: "npm", Title: "NPM Global Cache",
		Category: exoCore.CategoryPackageStore, CurrentPath: "/tmp/npm-cache",
		SizeBytes: 1024, IsActive: true, RedirectMethod: exoCore.RedirectNativeConfig,
	}
	linkItem := exoCore.CacheItem{
		ID: "cargo.registry", ToolName: "cargo", Title: "Cargo Registry",
		Category: exoCore.CategoryPackageStore, CurrentPath: "/tmp/cargo-registry",
		SizeBytes: 2048, IsActive: true, RedirectMethod: exoCore.RedirectSymlink,
	}

	engine := exoCore.NewEngine(&fakeProvider{name: "fake", items: []exoCore.CacheItem{configItem, linkItem}})
	app := nsgotest.Start(t, NewMain(engine), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:NPM Global Cache") })
	app.Settle()

	methodText := func(id string) string {
		views := app.Query("id:method-" + id)
		if len(views) != 1 {
			t.Fatalf("row %s shows no method:\n%s", id, app.Dump())
		}
		return views[0].Text
	}
	if got := methodText("npm.cache"); got != "Method: configuration" {
		t.Fatalf("npm method is %q", got)
	}
	if got := methodText("cargo.registry"); got != "Method: symbolic link" {
		t.Fatalf("cargo method is %q", got)
	}

	app.Type("id:target", t.TempDir())
	app.Settle()
	app.Press("id:migrate")
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("id:confirm") })

	dump := app.Dump()
	for _, want := range []string{"1 by tool configuration", "1 by symbolic link"} {
		if !strings.Contains(dump, want) {
			t.Fatalf("the confirmation does not say %q:\n%s", want, dump)
		}
	}
}
