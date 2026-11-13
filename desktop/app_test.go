package desktop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oneo-me/nsgo/core"
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
	// migrateErr makes every move stop with this error.
	migrateErr error
	// writes counts the calls that change the ledger. The ledger path is one
	// process-wide setting, so a call that still runs when a test ends writes
	// into the ledger of the next test.
	writes atomic.Int32
}

// waitForWrites waits until no call changes the ledger. A test that reads a
// step in the middle of a move defers this, so the move cannot outlive the
// test and taint the next one.
func (f *fakeProvider) waitForWrites(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for f.writes.Load() > 0 {
		if time.Now().After(deadline) {
			t.Fatal("a cache action did not finish")
		}
		time.Sleep(5 * time.Millisecond)
	}
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
	f.writes.Add(1)
	defer f.writes.Add(-1)
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
	if f.migrateErr != nil {
		return f.migrateErr
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
	f.writes.Add(1)
	defer f.writes.Add(-1)
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

// The window needs a title. Without one the system shows the name of the
// binary in the title bar, the Dock, and the window list.
func TestWindowSetsItsTitle(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))
	useVolumesRoot(t, t.TempDir())

	app := nsgotest.Start(t, NewMain(newFakeEngine(fakeCache())), nsgotest.Size(1100, 800))
	if app.Title != "Exodrive" {
		t.Fatalf("the window title is %q, want Exodrive", app.Title)
	}
}

// badgeText returns the state chip of a row, or "" when the row is not shown.
func badgeText(app *nsgotest.App, id string) string {
	views := app.Query("id:badge-" + id)
	if len(views) != 1 {
		return ""
	}
	return views[0].Text
}

// fakeCache is the cache every test moves.
func fakeCache() exoCore.CacheItem {
	return exoCore.CacheItem{
		ID:          "fake.cache",
		ToolName:    "fake",
		Title:       "Fake Cache",
		Category:    exoCore.CategoryPackageStore,
		CurrentPath: "/tmp/fake-cache",
		SizeBytes:   4096,
		IsActive:    true,
	}
}

// chooseTarget types a folder and waits until the window checks it. The main
// action offers the next step only after the check passes, so it is the signal
// that the folder is ready.
func chooseTarget(t *testing.T, app *nsgotest.App, target string) {
	t.Helper()
	app.Type("id:target", target)
	app.Settle()
	app.WaitUntil(2*time.Second, func() bool {
		return app.Exists("id:select-all") || app.Exists("id:migrate")
	})
}

// typeTarget fills the folder field without waiting for the check, for a
// test that expects the check to fail.
func typeTarget(app *nsgotest.App, target string) {
	app.Type("id:target", target)
	app.Settle()
}

// checkAll presses the main action that selects every cache that can move.
func checkAll(t *testing.T, app *nsgotest.App) {
	t.Helper()
	app.Press("id:select-all")
	app.Settle()
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("id:migrate") })
}

// TestWindowListsDetectedCaches mounts the real window and checks that a scan
// result reaches the list. The window must repaint after the scan goroutine
// posts its result.
func TestWindowListsDetectedCaches(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))

	app := nsgotest.Start(t, NewMain(newFakeEngine(fakeCache())), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Fake Cache") })
	app.Settle()

	if !app.Exists("text:4.0 KB") {
		t.Fatalf("the window did not show the cache size:\n%s", app.Dump())
	}
	if got := badgeText(app, "fake.cache"); got != "On this Mac" {
		t.Fatalf("row chip is %q, want On this Mac", got)
	}
	// Nothing moves before the window checks a folder.
	if app.Exists("id:migrate") {
		t.Fatalf("the window offers a move without a target folder:\n%s", app.Dump())
	}
	if !app.Exists("id:choose-target") {
		t.Fatalf("the window does not name the next step:\n%s", app.Dump())
	}
	// Nothing is selected by default, so one press cannot move a whole disk.
	if !app.Exists("id:select-fake.cache") || !app.Enabled("id:select-fake.cache") {
		t.Fatalf("a cache that can move is not selectable:\n%s", app.Dump())
	}
}

// TestWindowMigratesSelectedCaches drives the window as a user does: choose a
// folder, select the caches, press Move, confirm. The ledger must hold the
// record afterwards.
func TestWindowMigratesSelectedCaches(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))
	target := t.TempDir()
	item := fakeCache()

	app := nsgotest.Start(t, NewMain(newFakeEngine(item)), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Fake Cache") })
	app.Settle()

	chooseTarget(t, app, target)
	checkAll(t, app)

	app.Press("id:migrate")
	app.Settle()
	if !app.Exists("id:confirm") {
		t.Fatalf("Move did not ask for confirmation:\n%s", app.Dump())
	}
	if !strings.Contains(app.Dump(), "Fake Cache") {
		t.Fatalf("the confirmation does not name the cache:\n%s", app.Dump())
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

	app.WaitUntil(2*time.Second, func() bool { return badgeText(app, item.ID) == "On the drive" })
	if got := badgeText(app, item.ID); got != "On the drive" {
		t.Fatalf("row chip is %q, want On the drive:\n%s", got, app.Dump())
	}
}

// TestWindowMovesOnlyTheCheckedCaches checks the scope before the press: a
// cache that the user did not check stays on this Mac.
func TestWindowMovesOnlyTheCheckedCaches(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))
	target := t.TempDir()

	first := fakeCache()
	second := exoCore.CacheItem{
		ID: "fake.two", ToolName: "fake", Title: "Second Cache",
		Category: exoCore.CategoryPackageStore, CurrentPath: "/tmp/second",
		SizeBytes: 2048, IsActive: true,
	}
	engine := exoCore.NewEngine(&fakeProvider{name: "fake", items: []exoCore.CacheItem{first, second}})

	app := nsgotest.Start(t, NewMain(engine), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Second Cache") })
	app.Settle()

	chooseTarget(t, app, target)
	app.Press("id:select-fake.cache")
	app.Settle()
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("id:migrate") })

	app.Press("id:migrate")
	app.Settle()
	app.Press("id:confirm-move")

	app.WaitUntil(2*time.Second, func() bool {
		m, err := exoCore.LoadManifest()
		if err != nil {
			return false
		}
		_, ok := m.Records[first.ID]
		return ok
	})
	m, err := exoCore.LoadManifest()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Records[second.ID]; ok {
		t.Fatalf("a cache that was not checked moved:\n%s", app.Dump())
	}
}

// TestWindowRescanRecovers presses Rescan and waits for the window to settle.
// The scan runs in a goroutine and posts its result later, after the button has
// already repainted, so the post must reach the root scope.
func TestWindowRescanRecovers(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))

	app := nsgotest.Start(t, NewMain(newSlowFakeEngine(fakeCache(), 120*time.Millisecond)), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Fake Cache") })
	app.Settle()

	app.Press("id:rescan")
	app.Settle()
	if app.Enabled("id:rescan") {
		t.Fatalf("Rescan stays enabled while a check runs:\n%s", app.Dump())
	}

	app.WaitUntil(2*time.Second, func() bool { return app.Enabled("id:rescan") })
	if !app.Exists("text:Fake Cache") {
		t.Fatalf("the rescan lost the cache list:\n%s", app.Dump())
	}
}

// TestWindowRollsBackMigratedCache drives the way back: move a cache, press
// Move back on its card, confirm. The record must be gone and the card local
// again.
func TestWindowRollsBackMigratedCache(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))
	target := t.TempDir()
	item := fakeCache()

	app := nsgotest.Start(t, NewMain(newFakeEngine(item)), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Fake Cache") })
	app.Settle()

	// A local cache offers no way back, so its card carries no such action.
	if app.Exists("id:rollback-fake.cache") {
		t.Fatalf("a local cache offers a move back:\n%s", app.Dump())
	}

	chooseTarget(t, app, target)
	checkAll(t, app)
	app.Press("id:migrate")
	app.Settle()
	app.Press("id:confirm-move")
	app.WaitUntil(2*time.Second, func() bool { return badgeText(app, item.ID) == "On the drive" })

	// The action sits on the card, so no press hides it.
	if !app.Exists("id:rollback-fake.cache") {
		t.Fatalf("a moved cache offers no way back:\n%s", app.Dump())
	}

	app.Press("id:rollback-fake.cache")
	app.Settle()
	if !app.Exists("id:rollback-dialog") {
		t.Fatalf("Move back did not ask for confirmation:\n%s", app.Dump())
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
	app.WaitUntil(2*time.Second, func() bool { return badgeText(app, item.ID) == "On this Mac" })
}

// TestWindowDeletesAbandonedCache mounts the window, opens an unused cache,
// asks to delete it, and confirms. The folder must be gone.
func TestWindowDeletesAbandonedCache(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))

	orphanDir := filepath.Join(t.TempDir(), "Library", "Caches", "pnpm")
	if err := os.MkdirAll(orphanDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(orphanDir, "cache.dat"), []byte("dead"), 0644); err != nil {
		t.Fatal(err)
	}

	orphan := exoCore.CacheItem{
		ID:          "orphan.pnpm",
		ToolName:    "pnpm",
		Title:       "Legacy Local PNPM Cache",
		Category:    exoCore.CategoryOrphan,
		CurrentPath: orphanDir,
		SizeBytes:   4,
	}
	engine := exoCore.NewEngine(&fakeProvider{name: "fake", items: []exoCore.CacheItem{fakeCache(), orphan}})
	app := nsgotest.Start(t, NewMain(engine), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Legacy Local PNPM Cache") })
	app.Settle()

	if got := badgeText(app, orphan.ID); got != "Unused" {
		t.Fatalf("an unused cache shows %q, want Unused", got)
	}
	if app.Exists("id:clean-fake.cache") {
		t.Fatalf("a cache in use offers Delete:\n%s", app.Dump())
	}

	if !app.Exists("id:clean-orphan.pnpm") {
		t.Fatalf("an unused cache offers no Delete:\n%s", app.Dump())
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
		t.Fatalf("the unused cache still exists: %v", err)
	}
}

// A cache that Exodrive cannot move says so on its own line, and the window
// never offers to move it.
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

	if !strings.Contains(app.Dump(), "Yarn 4.1.0 reads cacheFolder") {
		t.Fatalf("the reason is not on the row:\n%s", app.Dump())
	}
	if app.Enabled("id:select-yarn.cache") {
		t.Fatalf("a cache that cannot move is selectable:\n%s", app.Dump())
	}
	if got := badgeText(app, blocked.ID); got != "Cannot move" {
		t.Fatalf("chip is %q, want Cannot move", got)
	}

	typeTarget(app, t.TempDir())
	app.WaitUntil(2*time.Second, func() bool {
		return strings.Contains(app.Dump(), "Nothing can move now")
	})
	if app.Exists("id:migrate") || app.Exists("id:select-all") {
		t.Fatalf("the window offers a move while every cache is blocked:\n%s", app.Dump())
	}
}

// A moved cache shows the folder that holds it. The old path alone leaves the
// user to guess where the data went.
func TestWindowShowsTheCopyOfAMovedCache(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))

	item := fakeCache()
	item.RedirectMethod = exoCore.RedirectSymlink
	destination := "/Volumes/SSD/exodrive/fake-cache"
	if err := exoCore.RecordMigration(exoCore.MigrationRecord{
		ID: item.ID, ToolName: item.ToolName, OriginalPath: item.CurrentPath,
		DestinationDir: destination, RedirectMethod: exoCore.RedirectSymlink,
		MigratedAt: time.Now(), SizeBytes: item.SizeBytes,
	}); err != nil {
		t.Fatal(err)
	}

	app := nsgotest.Start(t, NewMain(newFakeEngine(item)), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("id:destination-fake.cache") })

	views := app.Query("id:destination-fake.cache")
	if len(views) != 1 || !strings.Contains(views[0].Text, destination) {
		t.Fatalf("the row shows %+v, want the copy path", views)
	}
}

// The window must name a missing external drive. The folder picker lists only
// the internal disk, so the gauge is the only place that says what to do.
func TestWindowGuidesWhenNoVolumeIsConnected(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))
	useVolumesRoot(t, t.TempDir())

	app := nsgotest.Start(t, NewMain(newFakeEngine(fakeCache())), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Fake Cache") })
	app.WaitUntil(2*time.Second, func() bool {
		return strings.Contains(app.Dump(), "No external drive is connected.")
	})
}

// A connected drive shows its free space before the person chooses a folder.
// The size answers the first question the window gets: is there room.
func TestWindowShowsTheDriveBeforeAFolderIsChosen(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))
	root := useVolumesRoot(t, t.TempDir())
	if err := os.Mkdir(filepath.Join(root, "SSD"), 0755); err != nil {
		t.Fatal(err)
	}

	app := nsgotest.Start(t, NewMain(newFakeEngine(fakeCache())), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Fake Cache") })
	app.WaitUntil(2*time.Second, func() bool {
		dump := app.Dump()
		return strings.Contains(dump, " free") &&
			strings.Contains(dump, "SSD: choose a folder to move caches here.")
	})
}

// The check steps through every byte, so the window must name it. A progress
// line that still says "Moving" looks frozen.
func TestWindowShowsTheCheckStep(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))

	provider := &fakeProvider{
		name: "fake", items: []exoCore.CacheItem{fakeCache()},
		verifyPause: 800 * time.Millisecond,
	}
	// The test reads the step in the middle of the move, then ends. Wait for
	// the move, or its record lands in the next test's ledger.
	t.Cleanup(func() { provider.waitForWrites(t) })

	app := nsgotest.Start(t, NewMain(exoCore.NewEngine(provider)), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Fake Cache") })

	chooseTarget(t, app, t.TempDir())
	checkAll(t, app)
	app.Press("id:migrate")
	app.Settle()
	app.Press("id:confirm-move")

	app.WaitUntil(2*time.Second, func() bool {
		return strings.Contains(app.Dump(), "Checking the copy of Fake Cache")
	})
}

// The first run names the next step, and the folder field turns that step into
// the next one once the window checks the folder.
func TestWindowGuidesTheFirstRun(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))

	app := nsgotest.Start(t, NewMain(newFakeEngine(fakeCache())), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Fake Cache") })
	app.Settle()

	if !strings.Contains(app.Dump(), "Choose a folder for the caches") {
		t.Fatalf("the first run does not name the next step:\n%s", app.Dump())
	}
	if !app.Exists("id:choose-target") {
		t.Fatalf("the first run offers no folder action:\n%s", app.Dump())
	}

	chooseTarget(t, app, t.TempDir())
	app.Settle()
	if !strings.Contains(app.Dump(), "Select the caches to move") {
		t.Fatalf("the window does not ask for a selection after a ready folder:\n%s", app.Dump())
	}
}

// A long run must stop between moves, and it must say how far it got.
func TestWindowStopsAfterTheCancelledMove(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))

	first := fakeCache()
	first.Title = "First Cache"
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

	chooseTarget(t, app, t.TempDir())
	checkAll(t, app)
	app.Press("id:migrate")
	app.Settle()
	app.Press("id:confirm-move")

	app.WaitUntil(2*time.Second, func() bool { return app.Exists("id:cancel-move") })
	app.Press("id:cancel-move")
	// The window must answer the press before the current cache ends.
	app.WaitUntil(2*time.Second, func() bool {
		return strings.Contains(app.Dump(), "Stopping after this cache")
	})
	// Let the first move finish. The loop must see the stop before it starts
	// the second cache.
	close(hold)

	app.WaitUntil(3*time.Second, func() bool { return strings.Contains(app.Dump(), "Stopped.") })
	if got := badgeText(app, first.ID); got != "On the drive" {
		t.Fatalf("the first cache shows %q, want On the drive", got)
	}
	if got := badgeText(app, second.ID); got == "On the drive" {
		t.Fatalf("the second cache moved after the stop")
	}
}

// A row says what Exodrive changes so the program reaches the copy, in words a
// person who has never heard of a symbolic link can read.
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

	if !strings.Contains(app.Dump(), "Exodrive changes this program's setting.") {
		t.Fatalf("the npm row does not say what changes:\n%s", app.Dump())
	}
	if !strings.Contains(app.Dump(), "Exodrive links the old folder to the copy.") {
		t.Fatalf("the cargo row does not say what changes:\n%s", app.Dump())
	}
}

// A move that stops leaves a reason on its row and a next step next to it. The
// data stays, and the window says so.
func TestWindowShowsAMoveFailure(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))

	engine := exoCore.NewEngine(&fakeProvider{
		name: "fake", items: []exoCore.CacheItem{fakeCache()},
		migrateErr: errors.New("verification failed: /tmp/copy differs from the source file"),
	})

	app := nsgotest.Start(t, NewMain(engine), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Fake Cache") })

	chooseTarget(t, app, t.TempDir())
	checkAll(t, app)
	app.Press("id:migrate")
	app.Settle()
	app.Press("id:confirm-move")

	app.WaitUntil(2*time.Second, func() bool { return app.Exists("id:notice") })
	if got := badgeText(app, "fake.cache"); got != "Move failed" {
		t.Fatalf("the row chip is %q, want Move failed:\n%s", got, app.Dump())
	}

	app.WaitUntil(2*time.Second, func() bool { return app.Exists("id:retry-fake.cache") })

	dump := app.Dump()
	for _, want := range []string{"The copy did not match", "Exodrive deleted nothing.", "Try again"} {
		if !strings.Contains(dump, want) {
			t.Fatalf("the failure does not say %q:\n%s", want, dump)
		}
	}
}

// A folder that fails the check never reaches the move action. The window says
// what is wrong with it instead.
func TestWindowRefusesAFolderThatFailsTheCheck(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))

	app := nsgotest.Start(t, NewMain(newFakeEngine(fakeCache())), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Fake Cache") })
	app.Settle()

	app.Type("id:target", filepath.Join(t.TempDir(), "missing"))
	app.WaitUntil(2*time.Second, func() bool {
		return strings.Contains(app.Dump(), "does not exist")
	})

	if app.Exists("id:migrate") || app.Exists("id:select-all") {
		t.Fatalf("the window offers a move into a folder that failed the check:\n%s", app.Dump())
	}
	if !strings.Contains(app.Dump(), "Choose another folder") {
		t.Fatalf("the window does not name the next step:\n%s", app.Dump())
	}
}

// TestWindowRemindsWhenTheFolderIsOnThisMac keeps the reminder a reminder: the
// window says that a move here frees no space, and the move stays on offer.
func TestWindowRemindsWhenTheFolderIsOnThisMac(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home folder")
	}
	target, err := os.MkdirTemp(home, ".exodrive-target-")
	if err != nil {
		t.Skipf("home folder is not writable: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(target) })

	app := nsgotest.Start(t, NewMain(newFakeEngine(fakeCache())), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Fake Cache") })

	chooseTarget(t, app, target)

	reminder := "This folder is on this Mac. A move here frees no space."
	if !app.Exists("id:target-reminder") || !strings.Contains(app.Dump(), reminder) {
		t.Fatalf("the window does not say that a folder on this Mac frees no space:\n%s", app.Dump())
	}
	// The reminder must not act as a gate.
	if !app.Exists("id:select-all") {
		t.Fatalf("the reminder stopped the move:\n%s", app.Dump())
	}
	checkAll(t, app)
	app.Press("id:migrate")
	app.Settle()
	if !app.Exists("id:confirm") {
		t.Fatalf("the reminder stopped the move before the confirmation:\n%s", app.Dump())
	}
	// The confirmation reads the reminder again, where the move starts.
	if !strings.Contains(app.Dump(), reminder) {
		t.Fatalf("the confirmation does not repeat the reminder:\n%s", app.Dump())
	}
}

// TestWindowAsksTheSystemForAFolder leaves the folder picker to the system:
// the window asks for one folder, opens no picker of its own, and checks the
// folder the system answers with exactly as it checks a typed path.
func TestWindowAsksTheSystemForAFolder(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))
	target := t.TempDir()

	engine := newFakeEngine(fakeCache())
	var state *AppState
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		state = newState(cx, engine)
		return window(cx, state)
	}, nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Fake Cache") })

	app.Press("id:browse")
	app.Settle()
	if got := len(app.DirectoryPicks); got != 1 {
		t.Fatalf("the window asked the system for %d folders, not one", got)
	}
	if app.Exists("id:target-picker") {
		t.Fatalf("the window opened a folder picker of its own")
	}

	chooseFolder(state, target)
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("id:select-all") || app.Exists("id:migrate") })
	if got := state.Target.GetUntracked().Path; got != target {
		t.Fatalf("the picked folder is %q, not %q", got, target)
	}
}

// TestWindowFiltersByGroup keeps the rail a filter: a press on a group keeps
// that group's caches, and All brings them back.
func TestWindowFiltersByGroup(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))

	npm := fakeCache()
	npm.ID, npm.ToolName, npm.Title = "npm.cache", "npm", "NPM Cache"
	pip := fakeCache()
	pip.ID, pip.ToolName, pip.Title = "pip.cache", "pip", "Pip Cache"
	pip.CurrentPath = "/tmp/pip-cache"

	engine := exoCore.NewEngine(&fakeProvider{name: "fake", items: []exoCore.CacheItem{npm, pip}})
	app := nsgotest.Start(t, NewMain(engine), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:NPM Cache") })

	if !app.Exists("id:rail-Node") || !app.Exists("id:rail-Python") {
		t.Fatalf("the rail does not name the groups:\n%s", app.Dump())
	}

	app.Press("id:rail-Python")
	app.Settle()
	if !app.Exists("id:row-pip.cache") || app.Exists("id:row-npm.cache") {
		t.Fatalf("the Python group kept the wrong caches:\n%s", app.Dump())
	}

	app.Press("id:rail-all")
	app.Settle()
	if !app.Exists("id:row-npm.cache") || !app.Exists("id:row-pip.cache") {
		t.Fatalf("All did not bring every cache back:\n%s", app.Dump())
	}
}

// TestWindowFiltersByPlace keeps the two places apart: this Mac and the drive.
func TestWindowFiltersByPlace(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))

	local := fakeCache()
	moved := fakeCache()
	moved.ID, moved.Title, moved.CurrentPath = "moved.cache", "Moved Cache", "/tmp/moved-cache"
	if err := exoCore.RecordMigration(exoCore.MigrationRecord{
		ID: moved.ID, ToolName: moved.ToolName, OriginalPath: moved.CurrentPath,
		DestinationDir: "/Volumes/SSD/exodrive", RedirectMethod: exoCore.RedirectSymlink,
		MigratedAt: time.Now(), SizeBytes: moved.SizeBytes,
	}); err != nil {
		t.Fatal(err)
	}

	engine := exoCore.NewEngine(&fakeProvider{name: "fake", items: []exoCore.CacheItem{local, moved}})
	app := nsgotest.Start(t, NewMain(engine), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Moved Cache") })

	app.Press("id:place-this-mac")
	app.Settle()
	if !app.Exists("id:row-"+local.ID) || app.Exists("id:row-"+moved.ID) {
		t.Fatalf("On this Mac kept the wrong caches:\n%s", app.Dump())
	}

	app.Press("id:place-drive")
	app.Settle()
	if !app.Exists("id:row-"+moved.ID) || app.Exists("id:row-"+local.ID) {
		t.Fatalf("On the drive kept the wrong caches:\n%s", app.Dump())
	}

	app.Press("id:place-all")
	app.Settle()
	if !app.Exists("id:row-"+local.ID) || !app.Exists("id:row-"+moved.ID) {
		t.Fatalf("All did not bring every place back:\n%s", app.Dump())
	}
}

// TestWindowMovesOneCacheFromItsCard keeps a single move a first-class action:
// the card's Move asks for that cache alone and records it alone.
func TestWindowMovesOneCacheFromItsCard(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))
	target := t.TempDir()

	first := fakeCache()
	second := fakeCache()
	second.ID, second.Title, second.CurrentPath = "fake.two", "Second Cache", "/tmp/second"

	engine := exoCore.NewEngine(&fakeProvider{name: "fake", items: []exoCore.CacheItem{first, second}})
	app := nsgotest.Start(t, NewMain(engine), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Second Cache") })

	chooseTarget(t, app, target)
	app.Press("id:move-" + second.ID)
	app.Settle()
	if !app.Exists("id:confirm") {
		t.Fatalf("the card's Move did not ask for confirmation:\n%s", app.Dump())
	}
	// The confirmation holds the card's cache alone.
	if !strings.Contains(app.Dump(), "Move 1 cache?") {
		t.Fatalf("the card's Move asked for more than one cache:\n%s", app.Dump())
	}
	app.Press("id:confirm-move")

	app.WaitUntil(2*time.Second, func() bool {
		m, err := exoCore.LoadManifest()
		if err != nil {
			return false
		}
		_, ok := m.Records[second.ID]
		return ok
	})
	app.WaitUntil(2*time.Second, func() bool { return app.Enabled("id:rescan") })
	if got := badgeText(app, first.ID); got != "On this Mac" {
		t.Fatalf("a cache that was not chosen shows %q", got)
	}
	if got := badgeText(app, second.ID); got != "On the drive" {
		t.Fatalf("the chosen cache shows %q", got)
	}
}

// TestWindowDropsTheSelectionOnAFilter keeps the move honest: a cache that the
// filter hides never moves on a press that was meant for the cards on screen.
func TestWindowDropsTheSelectionOnAFilter(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))

	npm := fakeCache()
	npm.ID, npm.ToolName, npm.Title = "npm.cache", "npm", "NPM Cache"
	pip := fakeCache()
	pip.ID, pip.ToolName, pip.Title = "pip.cache", "pip", "Pip Cache"
	pip.CurrentPath = "/tmp/pip-cache"

	engine := exoCore.NewEngine(&fakeProvider{name: "fake", items: []exoCore.CacheItem{npm, pip}})
	app := nsgotest.Start(t, NewMain(engine), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:NPM Cache") })

	chooseTarget(t, app, t.TempDir())
	checkAll(t, app)
	if !app.Exists("id:migrate") {
		t.Fatalf("the window offers no move after Select all:\n%s", app.Dump())
	}

	app.Press("id:rail-Python")
	app.Settle()
	if app.Exists("id:migrate") {
		t.Fatalf("the selection survived a filter, so a hidden cache could move:\n%s", app.Dump())
	}
}

// TestWindowKeepsItsMarkWhileAnActionRuns mounts the mark in both of its
// states: still at rest, and moving while the window checks this Mac. The
// moving form starts an animation, so a broken branch would lose the mark.
func TestWindowKeepsItsMarkWhileAnActionRuns(t *testing.T) {
	app := nsgotest.Start(t, NewMain(newSlowFakeEngine(fakeCache(), 300*time.Millisecond)), nsgotest.Size(900, 720))
	if !app.Exists("id:brand-mark-moving") {
		t.Fatalf("the mark does not move while the check runs:\n%s", app.Dump())
	}
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Fake Cache") })
	app.Settle()
	if !app.Exists("id:brand-mark") || app.Exists("id:brand-mark-moving") {
		t.Fatalf("the mark still moves after the check:\n%s", app.Dump())
	}
	// The plate draws its two colors as a gradient. Without it the mark would
	// be an arrow on nothing, which no other test would see.
	mark := app.Query("id:brand-mark")
	if len(mark) != 1 || mark[0].Style.Effects == nil || len(mark[0].Style.Effects.Gradients) != 1 {
		t.Fatalf("the mark carries no gradient:\n%s", app.Dump())
	}
}

// recordMovedCopy writes the ledger record of a moved copy, and returns the
// folder that holds it. The digest is real, so a check can prove the copy.
func recordMovedCopy(t *testing.T, withDigest bool) string {
	t.Helper()
	copyDir := filepath.Join(t.TempDir(), "fake-cache")
	if err := os.MkdirAll(copyDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(copyDir, "blob"), []byte("cache-bytes"), 0644); err != nil {
		t.Fatal(err)
	}

	summary, err := fsutil.DigestPath(copyDir)
	if err != nil {
		t.Fatalf("DigestPath failed: %v", err)
	}
	rec := exoCore.MigrationRecord{
		ID:             "fake.cache",
		ToolName:       "fake",
		OriginalPath:   "/tmp/fake-cache",
		DestinationDir: copyDir,
		RedirectMethod: exoCore.RedirectSymlink,
		MigratedAt:     time.Now(),
		SizeBytes:      4096,
	}
	if withDigest {
		rec.TreeDigest = string(summary.Digest)
		rec.FileCount = summary.Files
		rec.TotalBytes = summary.Bytes
	}
	if err := exoCore.RecordMigration(rec); err != nil {
		t.Fatalf("RecordMigration failed: %v", err)
	}
	return copyDir
}

// mountedMovedCopy mounts the window over a moved cache and waits for its card.
func mountedMovedCopy(t *testing.T) *nsgotest.App {
	t.Helper()
	app := nsgotest.Start(t, NewMain(newFakeEngine(fakeCache())), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Fake Cache") })
	app.Settle()
	return app
}

// TestWindowChecksAMovedCopy gives the person the one fact the ledger can
// prove: the copy still holds the bytes that the move wrote.
func TestWindowChecksAMovedCopy(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))
	recordMovedCopy(t, true)

	app := mountedMovedCopy(t)
	if !app.Exists("id:check-fake.cache") {
		t.Fatalf("a moved cache offers no check:\n%s", app.Dump())
	}

	app.Press("id:check-fake.cache")
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:The copy matches the move.") })
	if !app.Exists("id:rollback-fake.cache") {
		t.Fatalf("the check replaced the way back:\n%s", app.Dump())
	}
}

func TestWindowReportsADamagedCopy(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))
	copyDir := recordMovedCopy(t, true)
	if err := os.WriteFile(filepath.Join(copyDir, "blob"), []byte("cache-bytez"), 0644); err != nil {
		t.Fatal(err)
	}

	app := mountedMovedCopy(t)
	app.Press("id:check-fake.cache")
	app.WaitUntil(2*time.Second, func() bool {
		return app.Exists("text:The copy differs from the move. Move the cache back, then move it again.")
	})
}

// TestWindowNeverCallsAnOldMoveChecked keeps the window honest about a record
// that holds no digest: it asks for a new move instead of a verdict.
func TestWindowNeverCallsAnOldMoveChecked(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))
	recordMovedCopy(t, false)

	app := mountedMovedCopy(t)
	app.Press("id:check-fake.cache")
	app.WaitUntil(2*time.Second, func() bool {
		return app.Exists("text:This move has no check. Move the cache back, then move it again.")
	})
	if app.Exists("text:The copy matches the move.") {
		t.Fatalf("the window called an unchecked copy good:\n%s", app.Dump())
	}
}

// TestWindowBlockedCacheNamesNoChange keeps a blocked card honest. The simulator
// folder carries a redirect method for the record, but Exodrive never moves it,
// so the card must not say that it makes a link.
func TestWindowBlockedCacheNamesNoChange(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))
	useVolumesRoot(t, t.TempDir())

	const reason = "Exodrive cannot move this folder. Keep this folder on this Mac."
	blocked := fakeCache()
	blocked.BlockedReason = reason
	blocked.RedirectMethod = exoCore.RedirectSymlink

	app := nsgotest.Start(t, NewMain(newFakeEngine(blocked)), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Fake Cache") })
	app.Settle()

	if !app.Exists("text:" + reason) {
		t.Fatalf("a blocked cache holds no reason:\n%s", app.Dump())
	}
	if app.Exists("text:" + englishText.SwitchLink) {
		t.Fatalf("a blocked cache claims a change:\n%s", app.Dump())
	}
}

// TestWindowRailNamesTheNewGroups keeps the rail readable: a group name comes
// from the window, so a new toolchain needs no catalog entry.
func TestWindowRailNamesTheNewGroups(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))
	useVolumesRoot(t, t.TempDir())

	dart := fakeCache()
	dart.ID, dart.ToolName, dart.Title = "dart.pub-cache", "dart", "Dart Pub Package Cache"
	brew := fakeCache()
	brew.ID, brew.ToolName, brew.Title = "homebrew.downloads", "homebrew", "Homebrew Downloads"

	engine := exoCore.NewEngine(&fakeProvider{name: "fake", items: []exoCore.CacheItem{dart, brew}})
	app := nsgotest.Start(t, NewMain(engine), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Dart Pub Package Cache") })
	app.Settle()

	for _, group := range []string{"Mobile", "Homebrew"} {
		// The quoted selector reads the whole text, so a card title that holds
		// the word does not stand in for the rail label.
		if !app.Exists(`text:"` + group + `"`) {
			t.Fatalf("the rail does not name the %s group:\n%s", group, app.Dump())
		}
	}
}
