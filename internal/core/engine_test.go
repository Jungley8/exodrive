package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"exodrive/internal/fsutil"
)

type MockProvider struct {
	name       string
	items      []CacheItem
	migrated   bool
	rollbacked bool
}

func (m *MockProvider) Name() string {
	return m.name
}

func (m *MockProvider) Detect(ctx context.Context) ([]CacheItem, error) {
	return m.items, nil
}

func (m *MockProvider) PreflightCheck(ctx context.Context, item CacheItem, targetRoot string) error {
	return nil
}

func (m *MockProvider) Migrate(ctx context.Context, item CacheItem, targetRoot string, progress fsutil.MigrateProgress) error {
	m.migrated = true
	return RecordMigration(MigrationRecord{
		ID:             item.ID,
		ToolName:       m.name,
		OriginalPath:   item.CurrentPath,
		DestinationDir: targetRoot,
		RedirectMethod: RedirectSymlink,
		MigratedAt:     time.Now(),
		SizeBytes:      item.SizeBytes,
	})
}

func (m *MockProvider) Rollback(ctx context.Context, item CacheItem) error {
	m.rollbacked = true
	return RemoveRecord(item.ID)
}

func TestEngineScanAndPreflight(t *testing.T) {
	t.Setenv(ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))
	ctx := context.Background()

	mockItem := CacheItem{
		ID:          "mock.test",
		ToolName:    "mock",
		Title:       "Mock Cache",
		Category:    CategoryPackageStore,
		CurrentPath: "/tmp/mock",
		SizeBytes:   1024 * 1024,
		IsActive:    true,
	}

	mockP := &MockProvider{
		name:  "mock",
		items: []CacheItem{mockItem},
	}

	engine := NewEngine(mockP)

	items, err := engine.ScanAll(ctx)
	if err != nil {
		t.Fatalf("ScanAll failed: %v", err)
	}
	if len(items) != 1 || items[0].ID != "mock.test" {
		t.Fatalf("unexpected scan results: %+v", items)
	}

	// Test filesystem safety guard
	if fsutil.IsSafeFilesystem("exfat") {
		t.Errorf("exfat should be deemed unsafe for dev caches")
	}
	if !fsutil.IsSafeFilesystem("apfs") {
		t.Errorf("apfs should be deemed safe")
	}
}

func TestEngineMigrateAndRollback(t *testing.T) {
	t.Setenv(ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))
	ctx := context.Background()
	targetDir := t.TempDir()

	mockItem := CacheItem{
		ID:          "test.item",
		ToolName:    "mock",
		Title:       "Test Item",
		Category:    CategoryBuildCache,
		CurrentPath: filepath.Join(targetDir, "src"),
		SizeBytes:   2048,
		IsActive:    true,
	}

	mockP := &MockProvider{
		name:  "mock",
		items: []CacheItem{mockItem},
	}

	engine := NewEngine(mockP)

	// Test Migrate
	if err := engine.Migrate(ctx, mockItem, targetDir, fsutil.MigrateProgress{}); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	if !mockP.migrated {
		t.Errorf("expected provider.Migrate to be invoked")
	}

	// Verify manifest entry
	m, err := LoadManifest()
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}
	if _, ok := m.Records[mockItem.ID]; !ok {
		t.Errorf("expected record in manifest for %s", mockItem.ID)
	}

	// Test Rollback
	if err := engine.Rollback(ctx, mockItem.ID); err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}
	if !mockP.rollbacked {
		t.Errorf("expected provider.Rollback to be invoked")
	}

	// Verify manifest removal
	m, _ = LoadManifest()
	if _, ok := m.Records[mockItem.ID]; ok {
		t.Errorf("expected record removed from manifest after rollback")
	}
}

func TestCleanOrphanDefensiveGuards(t *testing.T) {
	e := NewEngine()

	// Empty and whitespace paths
	if err := e.CleanOrphan(""); err == nil {
		t.Errorf("expected error on empty path, got nil")
	}
	if err := e.CleanOrphan("   "); err == nil {
		t.Errorf("expected error on whitespace path, got nil")
	}

	// Root paths
	if err := e.CleanOrphan("/"); err == nil {
		t.Errorf("expected error on root path '/', got nil")
	}
	if err := e.CleanOrphan("///"); err == nil {
		t.Errorf("expected error on root path '///', got nil")
	}

	// Relative paths
	if err := e.CleanOrphan("."); err == nil {
		t.Errorf("expected error on relative path '.', got nil")
	}
	if err := e.CleanOrphan("sub/orphan"); err == nil {
		t.Errorf("expected error on relative path 'sub/orphan', got nil")
	}

	// Shallow system directories
	if err := e.CleanOrphan("/Users"); err == nil {
		t.Errorf("expected error on shallow path '/Users', got nil")
	}
	if err := e.CleanOrphan("/tmp"); err == nil {
		t.Errorf("expected error on shallow path '/tmp', got nil")
	}
	// A mount root holds more than a cache.
	if err := e.CleanOrphan("/Volumes/ExternalDrive"); err == nil {
		t.Errorf("expected error on mount root '/Volumes/ExternalDrive', got nil")
	}

	// User home directory
	if home, err := os.UserHomeDir(); err == nil {
		if err := e.CleanOrphan(home); err == nil {
			t.Errorf("expected error on home directory %q, got nil", home)
		}
	}

	// A top-level home folder such as ~/Library holds unrelated data. Use a
	// temporary home so a guard regression cannot touch the real one.
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	if err := os.MkdirAll(filepath.Join(fakeHome, "Library"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := e.CleanOrphan(filepath.Join(fakeHome, "Library")); err == nil {
		t.Errorf("expected error on top-level home folder, got nil")
	}

	// Legitimate deeply nested orphan directory
	tmpDir := t.TempDir()
	orphanTarget := filepath.Join(tmpDir, "orphans", "go-build")
	if err := os.MkdirAll(orphanTarget, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(orphanTarget, "data.bin"), []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := e.CleanOrphan(orphanTarget); err != nil {
		t.Fatalf("CleanOrphan on valid target failed: %v", err)
	}
	if _, err := os.Stat(orphanTarget); !os.IsNotExist(err) {
		t.Errorf("expected orphanTarget to be deleted")
	}
}

// TestEngineUnknownProviderFailsClosed checks that an item whose toolchain has
// no provider stops the run instead of passing the preflight.
func TestEngineUnknownProviderFailsClosed(t *testing.T) {
	t.Setenv(ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))
	ctx := context.Background()

	engine := NewEngine(&MockProvider{name: "mock"})
	item := CacheItem{ID: "ghost.item", ToolName: "ghost", CurrentPath: "/tmp/ghost"}

	if err := engine.Preflight(ctx, item, t.TempDir()); err == nil {
		t.Fatalf("expected Preflight to fail for an unknown toolchain, got nil")
	}
	if err := engine.Migrate(ctx, item, t.TempDir(), fsutil.MigrateProgress{}); err == nil {
		t.Fatalf("expected Migrate to fail for an unknown toolchain, got nil")
	}
}

// TestEngineRefusesRepeatedMigration checks that the ledger guards a second
// migration of the same item.
func TestEngineRefusesRepeatedMigration(t *testing.T) {
	t.Setenv(ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))
	ctx := context.Background()
	targetDir := t.TempDir()

	item := CacheItem{
		ID:          "test.repeat",
		ToolName:    "mock",
		CurrentPath: filepath.Join(targetDir, "src"),
		SizeBytes:   1,
		IsActive:    true,
	}
	mockP := &MockProvider{name: "mock", items: []CacheItem{item}}
	engine := NewEngine(mockP)

	if err := engine.Migrate(ctx, item, targetDir, fsutil.MigrateProgress{}); err != nil {
		t.Fatalf("first Migrate failed: %v", err)
	}
	if err := engine.Migrate(ctx, item, targetDir, fsutil.MigrateProgress{}); err == nil {
		t.Fatalf("expected the second Migrate to be refused, got nil")
	}
}

// TestEngineScanMarksLedgerItems checks that a scan reports an item as
// migrated when the ledger holds its record.
func TestEngineScanMarksLedgerItems(t *testing.T) {
	t.Setenv(ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))
	ctx := context.Background()

	item := CacheItem{ID: "test.ledger", ToolName: "mock", CurrentPath: "/tmp/ledger", IsActive: true}
	engine := NewEngine(&MockProvider{name: "mock", items: []CacheItem{item}})

	if err := RecordMigration(MigrationRecord{
		ID:             item.ID,
		ToolName:       "mock",
		OriginalPath:   item.CurrentPath,
		DestinationDir: "/Volumes/External/exodrive-caches/mock",
		RedirectMethod: RedirectSymlink,
		MigratedAt:     time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	items, err := engine.ScanAll(ctx)
	if err != nil {
		t.Fatalf("ScanAll failed: %v", err)
	}
	if len(items) != 1 || !items[0].IsMigrated {
		t.Fatalf("expected the scanned item to be marked migrated, got %+v", items)
	}
}

type failingProvider struct{ name string }

func (f *failingProvider) Name() string { return f.name }

func (f *failingProvider) Detect(ctx context.Context) ([]CacheItem, error) {
	return nil, errors.New("scan failed")
}

func (f *failingProvider) PreflightCheck(ctx context.Context, item CacheItem, targetRoot string) error {
	return nil
}

func (f *failingProvider) Migrate(ctx context.Context, item CacheItem, targetRoot string, progress fsutil.MigrateProgress) error {
	return nil
}

func (f *failingProvider) Rollback(ctx context.Context, item CacheItem) error { return nil }

func TestScanAllReportsDetectErrors(t *testing.T) {
	t.Setenv(ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))

	good := &MockProvider{
		name:  "mock",
		items: []CacheItem{{ID: "mock.test", ToolName: "mock", Title: "Mock", CurrentPath: "/tmp/mock"}},
	}
	eng := NewEngine(good, &failingProvider{name: "broken"})

	items, err := eng.ScanAll(context.Background())
	if err == nil {
		t.Fatal("expected a detect error from the failing provider")
	}
	if !strings.Contains(err.Error(), "broken") {
		t.Fatalf("error does not name the failing provider: %v", err)
	}
	if len(items) != 1 || items[0].ID != "mock.test" {
		t.Fatalf("items from a working provider must survive: %+v", items)
	}
}
