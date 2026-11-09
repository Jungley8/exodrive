package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGetManifestPathDefault(t *testing.T) {
	t.Setenv(ManifestEnvVar, "")
	p := GetManifestPath()
	expectedSuffix := filepath.Join(".exodrive", "manifest.json")
	if !strings.HasSuffix(p, expectedSuffix) {
		t.Fatalf("expected path ending with %q, got %q", expectedSuffix, p)
	}
}

func TestGetManifestPathOverride(t *testing.T) {
	custom := filepath.Join(t.TempDir(), "custom-manifest.json")
	t.Setenv(ManifestEnvVar, custom)

	if got := GetManifestPath(); got != custom {
		t.Fatalf("expected manifest path %q, got %q", custom, got)
	}
}

func TestManifestLifecycleIsolated(t *testing.T) {
	tempLedger := filepath.Join(t.TempDir(), "sub", "ledger.json")
	t.Setenv(ManifestEnvVar, tempLedger)

	// Ensure empty load works
	m, err := LoadManifest()
	if err != nil {
		t.Fatalf("initial LoadManifest failed: %v", err)
	}
	if len(m.Records) != 0 {
		t.Fatalf("expected empty records, got %d", len(m.Records))
	}

	rec := MigrationRecord{
		ID:             "test.tool",
		ToolName:       "test",
		OriginalPath:   "/tmp/test",
		DestinationDir: "/tmp/ext",
		RedirectMethod: RedirectSymlink,
		MigratedAt:     time.Now(),
		SizeBytes:      4096,
	}

	if err := RecordMigration(rec); err != nil {
		t.Fatalf("RecordMigration failed: %v", err)
	}

	if _, err := os.Stat(tempLedger); err != nil {
		t.Fatalf("expected ledger file at %q to exist: %v", tempLedger, err)
	}

	loaded, err := LoadManifest()
	if err != nil {
		t.Fatalf("second LoadManifest failed: %v", err)
	}
	if _, ok := loaded.Records[rec.ID]; !ok {
		t.Fatalf("expected record %q to exist in ledger", rec.ID)
	}

	if err := RemoveRecord(rec.ID); err != nil {
		t.Fatalf("RemoveRecord failed: %v", err)
	}

	reloaded, err := LoadManifest()
	if err != nil {
		t.Fatalf("third LoadManifest failed: %v", err)
	}
	if _, ok := reloaded.Records[rec.ID]; ok {
		t.Fatalf("expected record %q to be deleted", rec.ID)
	}
}
