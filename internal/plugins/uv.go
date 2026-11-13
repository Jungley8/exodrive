package plugins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"exodrive/internal/core"
	"exodrive/internal/fsutil"
)

type UVProvider struct{}

func NewUVProvider() *UVProvider {
	return &UVProvider{}
}

func (p *UVProvider) Name() string {
	return "uv"
}

func (p *UVProvider) Detect(ctx context.Context) ([]core.CacheItem, error) {
	var items []core.CacheItem

	cacheDir, answered := toolQuery(ctx, "uv", "cache", "dir")

	home, _ := os.UserHomeDir()
	candidatePaths := []string{
		cacheDir,
		filepath.Join(home, "Library", "Caches", "uv"),
		filepath.Join(home, ".cache", "uv"),
	}

	blocked := ""
	if !answered {
		// A guessed folder would move a cache that uv does not read.
		blocked = toolMissingReason("uv", "uv cache dir")
	}

	for _, path := range candidatePaths {
		if path == "" {
			continue
		}
		if fi, err := os.Stat(path); err == nil && fi.IsDir() {
			size, _ := fsutil.DirSize(ctx, path)
			isSymlink := fsutil.IsSymlink(path)
			isMigrated := movedOffVolume(path)

			items = append(items, core.CacheItem{
				ID:             "uv.cache",
				ToolName:       "uv",
				Title:          "UV Package Cache",
				Category:       core.CategoryPackageStore,
				CurrentPath:    path,
				ConfiguredPath: cacheDir,
				SizeBytes:      size,
				IsActive:       true,
				IsMigrated:     isMigrated,
				IsSymlink:      isSymlink,
				RedirectMethod: core.RedirectSymlink,
				TargetSubdir:   "uv-cache",
				Warning:        "Cross-disk notice: venv creation uses copy fallback across separate volumes.",
				BlockedReason:  blocked,
			})
			break
		}
	}

	// Also detect UV Python toolchain installations
	pythonInstallDir := filepath.Join(home, ".local", "share", "uv", "python")
	if fi, err := os.Stat(pythonInstallDir); err == nil && fi.IsDir() {
		size, _ := fsutil.DirSize(ctx, pythonInstallDir)
		isSymlink := fsutil.IsSymlink(pythonInstallDir)
		isMigrated := movedOffVolume(pythonInstallDir)

		items = append(items, core.CacheItem{
			ID:             "uv.python",
			ToolName:       "uv",
			Title:          "UV Python Toolchains",
			Category:       core.CategoryPackageStore,
			CurrentPath:    pythonInstallDir,
			ConfiguredPath: pythonInstallDir,
			SizeBytes:      size,
			IsActive:       true,
			IsMigrated:     isMigrated,
			IsSymlink:      isSymlink,
			RedirectMethod: core.RedirectSymlink,
			TargetSubdir:   filepath.Join("uv", "python"),
		})
	}

	return items, nil
}

func (p *UVProvider) PreflightCheck(ctx context.Context, item core.CacheItem, targetRoot string) error {
	running, err := fsutil.CheckRunningProcesses("uv")
	if err != nil {
		return err
	}
	if len(running) > 0 {
		return fmt.Errorf("uv is running (PID: %s): stop active uv tasks before migration", running[0].PID)
	}
	// UV_CACHE_DIR wins over the link, so the switch would not take effect and
	// Exodrive would still delete the local cache.
	return envOverride("UV_CACHE_DIR")
}

func (p *UVProvider) Migrate(ctx context.Context, item core.CacheItem, targetRoot string, progress fsutil.MigrateProgress) error {
	return moveAndLink(p.Name(), item, filepath.Join(targetRoot, item.TargetSubdir), progress)
}

func (p *UVProvider) Rollback(ctx context.Context, item core.CacheItem) error {
	return rollbackLinked(item.ID)
}
