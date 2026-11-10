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
	// root instead. BUN_INSTALL names it; the default is ~/.bun.
	cacheDir, _ := toolQuery(ctx, "bun", "pm", "cache")

	home, _ := os.UserHomeDir()
	candidatePaths := []string{
		cacheDir,
		filepath.Join(bunInstallRoot(home), "install", "cache"),
		filepath.Join(home, ".bun", "install", "cache"),
		filepath.Join(home, "Library", "Caches", "bun"),
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
			isMigrated := strings.HasPrefix(path, "/Volumes/") || isSymlink

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
	dest := filepath.Join(targetRoot, item.TargetSubdir)
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}

	if item.IsSymlink {
		return fmt.Errorf("%s is already a symlink: nothing to migrate", item.CurrentPath)
	}

	if err := fsutil.CopyDir(item.CurrentPath, dest, item.SizeBytes, progress.Copy); err != nil {
		return fmt.Errorf("copy bun cache: %w", err)
	}
	if err := fsutil.VerifyTreeProgress(item.CurrentPath, dest, progress.Verify); err != nil {
		return fmt.Errorf("bun cache: %w", err)
	}

	if err := core.RecordMigration(core.MigrationRecord{
		ID:             item.ID,
		ToolName:       p.Name(),
		OriginalPath:   item.CurrentPath,
		DestinationDir: dest,
		RedirectMethod: core.RedirectSymlink,
		OriginalConfig: item.ConfiguredPath,
		MigratedAt:     time.Now(),
		SizeBytes:      item.SizeBytes,
	}); err != nil {
		return err
	}

	backupDir := item.CurrentPath + ".bak"
	if err := fsutil.ReplaceDirWithSymlink(item.CurrentPath, dest, backupDir); err != nil {
		return switchFailed(item.ID, err)
	}

	return discardLocalBackup(item.CurrentPath)
}

func (p *BunProvider) Rollback(ctx context.Context, item core.CacheItem) error {
	rec, err := loadRecord(item.ID)
	if err != nil {
		return err
	}

	if err := restoreSymlink(rec.OriginalPath, rec.DestinationDir); err != nil {
		return err
	}

	return core.RemoveRecord(item.ID)
}
