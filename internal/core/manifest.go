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

// LoadManifest reads the manifest file from ~/.exodrive/manifest.json (or EXODRIVE_MANIFEST_PATH).
func LoadManifest() (*Manifest, error) {
	manifestMu.Lock()
	defer manifestMu.Unlock()

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

// SaveManifest atomically writes the manifest to ~/.exodrive/manifest.json (or EXODRIVE_MANIFEST_PATH).
func SaveManifest(m *Manifest) error {
	manifestMu.Lock()
	defer manifestMu.Unlock()

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

// RecordMigration saves a migration record.
func RecordMigration(rec MigrationRecord) error {
	m, err := LoadManifest()
	if err != nil {
		return err
	}
	m.Records[rec.ID] = rec
	return SaveManifest(m)
}

// RemoveRecord removes a record upon rollback.
func RemoveRecord(id string) error {
	m, err := LoadManifest()
	if err != nil {
		return err
	}
	delete(m.Records, id)
	return SaveManifest(m)
}
