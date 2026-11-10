package plugins

import (
	"path/filepath"
	"testing"

	"exodrive/internal/core"
)

// The ledger is a file on disk. A rollback must refuse a record whose paths
// could delete the live cache or the home directory.
func TestValidateRecordRefusesUnsafePaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	good := core.MigrationRecord{
		ID:             "fake.cache",
		OriginalPath:   filepath.Join(home, "Library", "Caches", "fake"),
		DestinationDir: filepath.Join(t.TempDir(), "target", "fake-cache"),
	}

	if err := validateRecord(good); err != nil {
		t.Fatalf("a valid record was refused: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(core.MigrationRecord) core.MigrationRecord
	}{
		{"destination equals original", func(r core.MigrationRecord) core.MigrationRecord {
			r.DestinationDir = r.OriginalPath
			return r
		}},
		{"destination is the root", func(r core.MigrationRecord) core.MigrationRecord {
			r.DestinationDir = "/"
			return r
		}},
		{"destination is relative", func(r core.MigrationRecord) core.MigrationRecord {
			r.DestinationDir = "target/fake-cache"
			return r
		}},
		{"destination sits inside the original", func(r core.MigrationRecord) core.MigrationRecord {
			r.DestinationDir = filepath.Join(r.OriginalPath, "copy")
			return r
		}},
		{"original sits inside the destination", func(r core.MigrationRecord) core.MigrationRecord {
			r.OriginalPath = filepath.Join(r.DestinationDir, "inner")
			return r
		}},
		{"destination is the home directory", func(r core.MigrationRecord) core.MigrationRecord {
			r.DestinationDir = home
			return r
		}},
		{"destination is too close to the root", func(r core.MigrationRecord) core.MigrationRecord {
			r.DestinationDir = "/Volumes/External"
			return r
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateRecord(tc.mutate(good)); err == nil {
				t.Fatalf("expected a refusal for %s", tc.name)
			}
		})
	}
}

// loadRecord applies the check, so every rollback path runs it.
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
