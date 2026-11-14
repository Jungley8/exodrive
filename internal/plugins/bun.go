package plugins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"exodrive/internal/core"
	"exodrive/internal/fsutil"
)

type BunProvider struct{}

func NewBunProvider() *BunProvider {
	return &BunProvider{}
}

func (p *BunProvider) Name() string {
	return "bun"
}

func (p *BunProvider) Detect(ctx context.Context) ([]core.CacheItem, error) {
	var items []core.CacheItem

	// bun prints its cache only inside a project, so Exodrive reads the install
	// root instead. BUN_INSTALL names it; the default is ~/.bun. The install
	// cache is the folder that bun fills: an install writes the packages there.
	cacheDir, _ := toolQuery(ctx, "bun", "pm", "cache")

	home, _ := os.UserHomeDir()
	candidatePaths := []string{
		cacheDir,
		filepath.Join(bunInstallRoot(home), "install", "cache"),
		filepath.Join(home, ".bun", "install", "cache"),
	}

	seen := make(map[string]bool)
	for _, path := range candidatePaths {
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true

		if fi, err := os.Stat(path); err == nil && fi.IsDir() {
			size, _ := fsutil.DirSize(ctx, path)
			isSymlink := fsutil.IsSymlink(path)
			isMigrated := movedOffVolume(path)

			items = append(items, core.CacheItem{
				ID:             "bun.cache",
				ToolName:       "bun",
				Title:          "Bun Package Cache",
				Category:       core.CategoryPackageStore,
				CurrentPath:    path,
				ConfiguredPath: cacheDir,
				SizeBytes:      size,
				IsActive:       true,
				IsMigrated:     isMigrated,
				IsSymlink:      isSymlink,
				RedirectMethod: core.RedirectSymlink,
				TargetSubdir:   "bun-cache",
				Warning:        "Binary directory ~/.bun/bin remains on internal storage.",
			})
			break
		}
	}

	return items, nil
}

// bunInstallRoot returns the folder that holds the bun install. BUN_INSTALL
// names it, and ~/.bun is the default.
func bunInstallRoot(home string) string {
	if install := os.Getenv("BUN_INSTALL"); install != "" {
		return install
	}
	return filepath.Join(home, ".bun")
}

func (p *BunProvider) PreflightCheck(ctx context.Context, item core.CacheItem, targetRoot string) error {
	running, err := fsutil.CheckRunningProcesses("bun")
	if err != nil {
		return err
	}
	if len(running) > 0 {
		return fmt.Errorf("bun is running (PID: %s): stop active bun tasks before migration", running[0].PID)
	}
	// BUN_INSTALL_CACHE_DIR wins over the link, so the switch would not take
	// effect and Exodrive would still delete the local cache.
	return envOverride("BUN_INSTALL_CACHE_DIR")
}

func (p *BunProvider) Migrate(ctx context.Context, item core.CacheItem, targetRoot string, progress fsutil.MigrateProgress) error {
	return moveAndLink(p.Name(), item, filepath.Join(targetRoot, item.TargetSubdir), progress)
}

func (p *BunProvider) Rollback(ctx context.Context, item core.CacheItem) error {
	return rollbackLinked(item.ID)
}
