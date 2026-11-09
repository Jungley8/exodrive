package core

import (
	"context"
	"os"
	"path/filepath"
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

func (m *MockProvider) Migrate(ctx context.Context, item CacheItem, targetRoot string, onProgress fsutil.CopyProgressCallback) error {
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
	if err := engine.Migrate(ctx, mockItem, targetDir, nil); err != nil {
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

	// User home directory
	if home, err := os.UserHomeDir(); err == nil {
		if err := e.CleanOrphan(home); err == nil {
			t.Errorf("expected error on home directory %q, got nil", home)
		}
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
