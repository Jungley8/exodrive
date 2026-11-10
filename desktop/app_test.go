package desktop

import (
	"context"
	"path/filepath"
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

func (f *fakeProvider) Migrate(ctx context.Context, item exoCore.CacheItem, targetRoot string, onProgress fsutil.CopyProgressCallback) error {
	if onProgress != nil {
		onProgress(1, 1)
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
