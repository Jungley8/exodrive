package plugins

import (
	"path/filepath"
	"testing"

	"exodrive/internal/core"
)

// loadRecord applies the check, so every rollback path runs it. The check
// itself lives in core, because a keep-local runs it too.
func TestLoadRecordRefusesUnsafeRecord(t *testing.T) {
	t.Setenv(core.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))

	if err := core.RecordMigration(core.MigrationRecord{
		ID:             "fake.cache",
		OriginalPath:   "/tmp/fake-cache",
		DestinationDir: "/",
		RedirectMethod: core.RedirectSymlink,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := loadRecord("fake.cache"); err == nil {
		t.Fatal("expected loadRecord to refuse a record with a root destination")
	}
}
