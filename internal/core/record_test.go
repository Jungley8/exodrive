package core

import (
	"path/filepath"
	"testing"
)

// The ledger is a file on disk. A rollback and a keep-local must refuse a
// record whose paths could delete the live cache or the home directory.
func TestValidateRecordRefusesUnsafePaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	good := MigrationRecord{
		ID:             "fake.cache",
		OriginalPath:   filepath.Join(home, "Library", "Caches", "fake"),
		DestinationDir: filepath.Join(t.TempDir(), "target", "fake-cache"),
	}

	if err := ValidateRecord(good); err != nil {
		t.Fatalf("a valid record was refused: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(MigrationRecord) MigrationRecord
	}{
		{"destination equals original", func(r MigrationRecord) MigrationRecord {
			r.DestinationDir = r.OriginalPath
			return r
		}},
		{"destination is the root", func(r MigrationRecord) MigrationRecord {
			r.DestinationDir = "/"
			return r
		}},
		{"destination is relative", func(r MigrationRecord) MigrationRecord {
			r.DestinationDir = "target/fake-cache"
			return r
		}},
		{"destination sits inside the original", func(r MigrationRecord) MigrationRecord {
			r.DestinationDir = filepath.Join(r.OriginalPath, "copy")
			return r
		}},
		{"original sits inside the destination", func(r MigrationRecord) MigrationRecord {
			r.OriginalPath = filepath.Join(r.DestinationDir, "inner")
			return r
		}},
		{"destination is the home directory", func(r MigrationRecord) MigrationRecord {
			r.DestinationDir = home
			return r
		}},
		{"destination is too close to the root", func(r MigrationRecord) MigrationRecord {
			r.DestinationDir = "/Volumes/External"
			return r
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateRecord(tc.mutate(good)); err == nil {
				t.Fatalf("expected a refusal for %s", tc.name)
			}
		})
	}
}
