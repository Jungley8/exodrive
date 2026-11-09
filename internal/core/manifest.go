package core

import (
	"encoding/json"
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
func saveManifestLocked(m *Manifest) error {
	p := GetManifestPath()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}

	m.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}

	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
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

// ModifyManifest executes fn within an exclusive transaction, holding manifestMu
// across the load, mutation, and save cycle.
func ModifyManifest(fn func(m *Manifest) error) error {
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

// RecordMigration saves a migration record atomically under manifestMu.
func RecordMigration(rec MigrationRecord) error {
	return ModifyManifest(func(m *Manifest) error {
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
