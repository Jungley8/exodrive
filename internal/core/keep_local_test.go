package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"exodrive/internal/fsutil"
)

// keepLocalFixture builds a move whose tool wrote a real folder again: the
// original path holds new data, and the drive copy holds the old data.
type keepLocalFixture struct {
	original string
	copy     string
}

func newKeepLocalFixture(t *testing.T) keepLocalFixture {
	t.Helper()
	t.Setenv(ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))
	root := t.TempDir()

	f := keepLocalFixture{
		original: filepath.Join(root, "home", "Library", "Caches", "fake"),
		copy:     filepath.Join(root, "drive", "target", "fake-cache"),
	}
	for path, name := range map[string]string{f.original: "new", f.copy: "old"} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, name), []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := RecordMigration(MigrationRecord{
		ID:             "fake.cache",
		ToolName:       "fake",
		OriginalPath:   f.original,
		DestinationDir: f.copy,
		RedirectMethod: RedirectSymlink,
		MigratedAt:     time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	return f
}

// TestKeepLocalRemovesTheCopyAndTheRecord is the case a tool creates: the
// folder on this Mac is the one to keep, so the copy and the record go.
func TestKeepLocalRemovesTheCopyAndTheRecord(t *testing.T) {
	f := newKeepLocalFixture(t)

	if err := NewEngine().KeepLocal("fake.cache"); err != nil {
		t.Fatalf("KeepLocal refused the recovery: %v", err)
	}
	if _, err := os.Stat(f.copy); !os.IsNotExist(err) {
		t.Fatalf("the copy on the drive is still there: %v", err)
	}
	kept, err := os.ReadFile(filepath.Join(f.original, "new"))
	if err != nil || string(kept) != "new" {
		t.Fatalf("the local folder changed: %q %v", kept, err)
	}
	m, err := LoadManifest()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Records["fake.cache"]; ok {
		t.Fatal("the ledger still holds the record")
	}
}

// TestKeepLocalRefusesWhileTheLinkStands keeps the normal rollback in charge
// while the link is still there.
func TestKeepLocalRefusesWhileTheLinkStands(t *testing.T) {
	f := newKeepLocalFixture(t)
	if err := os.RemoveAll(f.original); err != nil {
		t.Fatal(err)
	}
	if err := fsutil.AtomicSymlink(f.copy, f.original); err != nil {
		t.Fatal(err)
	}

	err := NewEngine().KeepLocal("fake.cache")
	if err == nil {
		t.Fatal("KeepLocal accepted a path that still links to the copy")
	}
	if !strings.Contains(err.Error(), "rollback") {
		t.Fatalf("the error does not name the rollback: %v", err)
	}
	if _, err := os.Stat(f.copy); err != nil {
		t.Fatalf("the copy changed: %v", err)
	}
	if m, _ := LoadManifest(); len(m.Records) != 1 {
		t.Fatalf("the ledger lost the record: %+v", m.Records)
	}
}

// TestKeepLocalRefusesWhenTheOriginalIsGone keeps a missing original with the
// rollback, which can still restore it.
func TestKeepLocalRefusesWhenTheOriginalIsGone(t *testing.T) {
	f := newKeepLocalFixture(t)
	if err := os.RemoveAll(f.original); err != nil {
		t.Fatal(err)
	}

	if err := NewEngine().KeepLocal("fake.cache"); err == nil {
		t.Fatal("KeepLocal accepted a missing original path")
	}
	if _, err := os.Stat(f.copy); err != nil {
		t.Fatalf("the copy changed: %v", err)
	}
}

// TestKeepLocalRefusesADetachedDrive keeps the record while the copy is out of
// reach. A dropped record would leave data on the drive with no bookkeeping.
func TestKeepLocalRefusesADetachedDrive(t *testing.T) {
	f := newKeepLocalFixture(t)
	if err := os.RemoveAll(filepath.Join(filepath.Dir(f.copy))); err != nil {
		t.Fatal(err)
	}

	err := NewEngine().KeepLocal("fake.cache")
	if err == nil {
		t.Fatal("KeepLocal accepted an unreachable drive")
	}
	if !strings.Contains(err.Error(), "not reachable") {
		t.Fatalf("the error does not name the drive: %v", err)
	}
	if _, err := os.Stat(f.original); err != nil {
		t.Fatalf("the local folder changed: %v", err)
	}
	if m, _ := LoadManifest(); len(m.Records) != 1 {
		t.Fatalf("the ledger lost the record: %+v", m.Records)
	}
}

// TestKeepLocalDropsTheRecordWhenTheCopyIsGone covers a copy that someone
// deleted by hand: the drive answers, so the record can go.
func TestKeepLocalDropsTheRecordWhenTheCopyIsGone(t *testing.T) {
	f := newKeepLocalFixture(t)
	if err := os.RemoveAll(f.copy); err != nil {
		t.Fatal(err)
	}

	if err := NewEngine().KeepLocal("fake.cache"); err != nil {
		t.Fatalf("KeepLocal refused a deleted copy: %v", err)
	}
	if m, _ := LoadManifest(); len(m.Records) != 0 {
		t.Fatalf("the ledger still holds %d records", len(m.Records))
	}
}

// TestKeepLocalRefusesWithoutARecord keeps a stray call from deleting anything.
func TestKeepLocalRefusesWithoutARecord(t *testing.T) {
	t.Setenv(ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))

	if err := NewEngine().KeepLocal("ghost.cache"); err == nil {
		t.Fatal("KeepLocal accepted an item with no record")
	}
}

// TestKeepLocalRefusesAnUnsafeRecord runs the ledger check before any delete.
func TestKeepLocalRefusesAnUnsafeRecord(t *testing.T) {
	t.Setenv(ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))
	source := filepath.Join(t.TempDir(), "local", "fake")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	if err := RecordMigration(MigrationRecord{
		ID:             "fake.cache",
		OriginalPath:   source,
		DestinationDir: "/",
		RedirectMethod: RedirectSymlink,
	}); err != nil {
		t.Fatal(err)
	}

	if err := NewEngine().KeepLocal("fake.cache"); err == nil {
		t.Fatal("KeepLocal accepted a record with a root destination")
	}
}
