package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

func TestManifestConcurrentRecordMigration(t *testing.T) {
	tempLedger := filepath.Join(t.TempDir(), "concurrent_ledger.json")
	t.Setenv(ManifestEnvVar, tempLedger)

	const total = 50
	var wg sync.WaitGroup
	errCh := make(chan error, total)

	for i := 0; i < total; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			rec := MigrationRecord{
				ID:             fmt.Sprintf("tool.%d", id),
				ToolName:       "tool",
				OriginalPath:   fmt.Sprintf("/tmp/src/%d", id),
				DestinationDir: fmt.Sprintf("/tmp/dst/%d", id),
				RedirectMethod: RedirectSymlink,
				MigratedAt:     time.Now(),
				SizeBytes:      int64(id * 1024),
			}
			if err := RecordMigration(rec); err != nil {
				errCh <- err
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrent RecordMigration failed: %v", err)
	}

	loaded, err := LoadManifest()
	if err != nil {
		t.Fatalf("failed to load manifest: %v", err)
	}

	if len(loaded.Records) != total {
		t.Fatalf("lost updates detected! expected %d records, got %d", total, len(loaded.Records))
	}
}

func TestRecordMigrationRefusesDuplicate(t *testing.T) {
	t.Setenv(ManifestEnvVar, filepath.Join(t.TempDir(), "duplicate_ledger.json"))

	first := MigrationRecord{
		ID:             "npm.cache",
		ToolName:       "npm",
		OriginalPath:   "/home/user/.npm",
		DestinationDir: "/Volumes/External/npm-cache",
		RedirectMethod: RedirectNativeConfig,
		MigratedAt:     time.Now(),
	}
	if err := RecordMigration(first); err != nil {
		t.Fatalf("first RecordMigration failed: %v", err)
	}

	second := first
	second.DestinationDir = "/Volumes/Other/npm-cache"
	if err := RecordMigration(second); err == nil {
		t.Fatal("expected RecordMigration to refuse a duplicate id")
	}

	loaded, err := LoadManifest()
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}
	if got := loaded.Records["npm.cache"].DestinationDir; got != first.DestinationDir {
		t.Fatalf("duplicate record overwrote the ledger: destination is %q, want %q", got, first.DestinationDir)
	}
}

// TestManifestRefusesANewerVersion stops an old build from rewriting a ledger
// that a newer build wrote, because the rewrite would drop fields it cannot read.
func TestManifestRefusesANewerVersion(t *testing.T) {
	ledger := filepath.Join(t.TempDir(), "newer.json")
	t.Setenv(ManifestEnvVar, ledger)

	data := fmt.Sprintf(`{"version": %d, "records": {}}`, ManifestVersion+1)
	if err := os.WriteFile(ledger, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(); err == nil {
		t.Fatal("LoadManifest accepted a ledger from a newer version")
	}
}

// TestManifestUpgradesAVersionOneLedger covers a record that predates the
// digest. The load accepts it, and the next save writes version 2.
func TestManifestUpgradesAVersionOneLedger(t *testing.T) {
	ledger := filepath.Join(t.TempDir(), "old.json")
	t.Setenv(ManifestEnvVar, ledger)

	old := `{"version": 1, "records": {"npm.cache": {
		"id": "npm.cache",
		"tool_name": "npm",
		"original_path": "/home/user/.npm",
		"destination_dir": "/Volumes/External/npm-cache",
		"redirect_method": "native_config",
		"migrated_at": "2026-01-01T00:00:00Z",
		"size_bytes": 4096
	}}}`
	if err := os.WriteFile(ledger, []byte(old), 0644); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadManifest()
	if err != nil {
		t.Fatalf("LoadManifest failed on a version 1 ledger: %v", err)
	}
	if loaded.Records["npm.cache"].TreeDigest != "" {
		t.Fatal("a version 1 record gained a digest it never measured")
	}

	if err := RecordMigration(MigrationRecord{
		ID:             "cargo.registry",
		ToolName:       "cargo",
		OriginalPath:   "/home/user/.cargo/registry",
		DestinationDir: "/Volumes/External/cargo-registry",
		RedirectMethod: RedirectSymlink,
		MigratedAt:     time.Now(),
	}); err != nil {
		t.Fatalf("RecordMigration failed: %v", err)
	}

	data, err := os.ReadFile(ledger)
	if err != nil {
		t.Fatal(err)
	}
	var written Manifest
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatal(err)
	}
	if written.Version != ManifestVersion {
		t.Fatalf("the ledger version is %d, want %d", written.Version, ManifestVersion)
	}
	if len(written.Records) != 2 {
		t.Fatalf("the ledger holds %d records, want 2", len(written.Records))
	}
}
