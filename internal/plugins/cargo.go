package plugins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

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
			size, _ := fsutil.DirSize(ctx, path)
			isSymlink := fsutil.IsSymlink(path)
			isMigrated := movedOffVolume(path)

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
	running, err := fsutil.CheckRunningProcesses("cargo", "rustc")
	if err != nil {
		return err
	}
	if len(running) > 0 {
		return fmt.Errorf("cargo or rustc is running (PID: %s): stop active builds before migration", running[0].PID)
	}
	return nil
}

func (p *CargoProvider) Migrate(ctx context.Context, item core.CacheItem, targetRoot string, progress fsutil.MigrateProgress) error {
	return moveAndLink(p.Name(), item, filepath.Join(targetRoot, item.TargetSubdir), progress)
}

func (p *CargoProvider) Rollback(ctx context.Context, item core.CacheItem) error {
	return rollbackLinked(item.ID)
}
