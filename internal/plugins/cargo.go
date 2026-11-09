package plugins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"exodrive/internal/core"
	"exodrive/internal/fsutil"
)

type CargoProvider struct{}

func NewCargoProvider() *CargoProvider {
	return &CargoProvider{}
}

func (p *CargoProvider) Name() string {
	return "cargo"
}

func (p *CargoProvider) Detect(ctx context.Context) ([]core.CacheItem, error) {
	var items []core.CacheItem

	cargoHome := os.Getenv("CARGO_HOME")
	if cargoHome == "" {
		home, _ := os.UserHomeDir()
		cargoHome = filepath.Join(home, ".cargo")
	}

	subdirs := []struct {
		id       string
		name     string
		sub      string
		target   string
		category core.CacheCategory
	}{
		{
			id:       "cargo.registry",
			name:     "Cargo Crate Registry",
			sub:      "registry",
			target:   filepath.Join("cargo", "registry"),
			category: core.CategoryPackageStore,
		},
		{
			id:       "cargo.git",
			name:     "Cargo Git Repositories",
			sub:      "git",
			target:   filepath.Join("cargo", "git"),
			category: core.CategoryBuildCache,
		},
	}

	for _, s := range subdirs {
		path := filepath.Join(cargoHome, s.sub)
		if fi, err := os.Stat(path); err == nil && fi.IsDir() {
			size, _ := fsutil.DirSize(path)
			isSymlink := fsutil.IsSymlink(path)
			isMigrated := strings.HasPrefix(path, "/Volumes/") || isSymlink

			items = append(items, core.CacheItem{
				ID:             s.id,
				ToolName:       "cargo",
				Title:          s.name,
				Category:       s.category,
				CurrentPath:    path,
				ConfiguredPath: cargoHome,
				SizeBytes:      size,
				IsActive:       true,
				IsMigrated:     isMigrated,
				IsSymlink:      isSymlink,
				RedirectMethod: core.RedirectSymlink,
				TargetSubdir:   s.target,
				Warning:        "Binary directory ~/.cargo/bin remains on internal storage.",
			})
		}
	}

	return items, nil
}

func (p *CargoProvider) PreflightCheck(ctx context.Context, item core.CacheItem, targetRoot string) error {
	running := fsutil.CheckRunningProcesses("cargo", "rustc")
	if len(running) > 0 {
		return fmt.Errorf("cargo or rustc is running (PID: %s): stop active builds before migration", running[0].PID)
	}
	return nil
}

func (p *CargoProvider) Migrate(ctx context.Context, item core.CacheItem, targetRoot string, onProgress fsutil.CopyProgressCallback) error {
	dest := filepath.Join(targetRoot, item.TargetSubdir)
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}

	if !item.IsSymlink {
		if err := fsutil.CopyDir(item.CurrentPath, dest, item.SizeBytes, onProgress); err != nil {
			return fmt.Errorf("copy cargo %s: %w", item.Title, err)
		}

		backupDir := item.CurrentPath + ".bak"
		if err := fsutil.ReplaceDirWithSymlink(item.CurrentPath, dest, backupDir); err != nil {
			return err
		}
	}

	return core.RecordMigration(core.MigrationRecord{
		ID:             item.ID,
		ToolName:       p.Name(),
		OriginalPath:   item.CurrentPath,
		DestinationDir: dest,
		RedirectMethod: core.RedirectSymlink,
		OriginalConfig: item.ConfiguredPath,
		MigratedAt:     time.Now(),
		SizeBytes:      item.SizeBytes,
	})
}

func (p *CargoProvider) Rollback(ctx context.Context, item core.CacheItem) error {
	m, err := core.LoadManifest()
	if err != nil {
		return err
	}
	rec, ok := m.Records[item.ID]
	if !ok {
		return fmt.Errorf("no migration record for %s", item.ID)
	}

	backupDir := rec.OriginalPath + ".bak"
	if fi, err := os.Stat(backupDir); err == nil && fi.IsDir() {
		_ = os.Remove(rec.OriginalPath) // remove symlink
		_ = os.Rename(backupDir, rec.OriginalPath)
	}

	return core.RemoveRecord(item.ID)
}
