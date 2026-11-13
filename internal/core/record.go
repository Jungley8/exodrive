package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateRecord refuses a ledger record whose paths could delete the wrong
// data. The ledger is a file on disk, so every rollback and every keep-local
// runs this check before it touches a folder.
func ValidateRecord(rec MigrationRecord) error {
	original := filepath.Clean(rec.OriginalPath)
	destination := filepath.Clean(rec.DestinationDir)

	if original == destination {
		return fmt.Errorf("ledger record %s: the original path equals the destination", rec.ID)
	}
	for name, path := range map[string]string{"original path": original, "destination": destination} {
		if !filepath.IsAbs(path) || path == string(filepath.Separator) {
			return fmt.Errorf("ledger record %s: unsafe %s %q", rec.ID, name, path)
		}
	}
	if within(original, destination) || within(destination, original) {
		return fmt.Errorf("ledger record %s: %s and %s hold each other", rec.ID, original, destination)
	}
	if home, err := os.UserHomeDir(); err == nil && destination == filepath.Clean(home) {
		return fmt.Errorf("ledger record %s: the destination is the home directory", rec.ID)
	}
	if strings.Count(strings.Trim(destination, string(filepath.Separator)), string(filepath.Separator)) < 2 {
		return fmt.Errorf("ledger record %s: destination %s is too close to the root", rec.ID, destination)
	}
	return nil
}

// within reports whether child sits below parent.
func within(child, parent string) bool {
	return strings.HasPrefix(child, parent+string(filepath.Separator))
}
