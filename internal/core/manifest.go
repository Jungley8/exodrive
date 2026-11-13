package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ManifestEnvVar is the environment variable used to override the default manifest path.
const ManifestEnvVar = "EXODRIVE_MANIFEST_PATH"

var manifestMu sync.Mutex

// GetManifestPath returns the active manifest ledger file path.
// If EXODRIVE_MANIFEST_PATH is set, its trimmed value is used.
// Otherwise, it defaults to ~/.exodrive/manifest.json.
func GetManifestPath() string {
	if custom := strings.TrimSpace(os.Getenv(ManifestEnvVar)); custom != "" {
		return custom
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".exodrive", "manifest.json")
}

// StatePath returns a file that the window keeps beside the ledger. A run that
// moves the ledger moves these files with it.
func StatePath(name string) string {
	return filepath.Join(filepath.Dir(GetManifestPath()), name)
}

// loadManifestLocked reads the manifest from disk. Caller MUST hold manifestMu.
func loadManifestLocked() (*Manifest, error) {
	p := GetManifestPath()
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &Manifest{
				Version: 1,
				Records: make(map[string]MigrationRecord),
			}, nil
		}
		return nil, err
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m.Records == nil {
		m.Records = make(map[string]MigrationRecord)
	}
	return &m, nil
}

// saveManifestLocked atomically writes the manifest to disk. Caller MUST hold manifestMu.
// The temporary file is flushed before the rename, and the directory is flushed
// after it, so a power loss cannot leave an empty or half-written ledger.
func saveManifestLocked(m *Manifest) error {
	p := GetManifestPath()
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	m.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(p)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Chmod(0644); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, p); err != nil {
		cleanup()
		return err
	}

	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	return syncDir(d)
}

// LoadManifest reads the manifest file from ~/.exodrive/manifest.json (or EXODRIVE_MANIFEST_PATH).
func LoadManifest() (*Manifest, error) {
	manifestMu.Lock()
	defer manifestMu.Unlock()
	return loadManifestLocked()
}

// SaveManifest atomically writes the manifest to ~/.exodrive/manifest.json (or EXODRIVE_MANIFEST_PATH).
func SaveManifest(m *Manifest) error {
	manifestMu.Lock()
	defer manifestMu.Unlock()
	return saveManifestLocked(m)
}

// ModifyManifest executes fn within an exclusive transaction, holding the
// in-process mutex and the cross-process file lock across the load, mutation,
// and save cycle.
func ModifyManifest(fn func(m *Manifest) error) error {
	unlock, err := lockManifest()
	if err != nil {
		return err
	}
	defer unlock()

	manifestMu.Lock()
	defer manifestMu.Unlock()

	m, err := loadManifestLocked()
	if err != nil {
		return err
	}
	if err := fn(m); err != nil {
		return err
	}
	return saveManifestLocked(m)
}

// RecordMigration saves a migration record atomically under the ledger lock.
// It refuses a record that the ledger already holds, so two runs cannot
// overwrite the paths that a rollback needs.
func RecordMigration(rec MigrationRecord) error {
	return ModifyManifest(func(m *Manifest) error {
		if existing, ok := m.Records[rec.ID]; ok {
			return fmt.Errorf("ledger already records %s at %s: run 'exodrive rollback %s' first", rec.ID, existing.DestinationDir, rec.ID)
		}
		m.Records[rec.ID] = rec
		return nil
	})
}

// RemoveRecord removes a record upon rollback atomically under manifestMu.
func RemoveRecord(id string) error {
	return ModifyManifest(func(m *Manifest) error {
		delete(m.Records, id)
		return nil
	})
}
