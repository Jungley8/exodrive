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

type PipProvider struct{}

func NewPipProvider() *PipProvider {
	return &PipProvider{}
}

func (p *PipProvider) Name() string {
	return "pip"
}

func (p *PipProvider) Detect(ctx context.Context) ([]core.CacheItem, error) {
	var items []core.CacheItem

	var cacheDir string
	answered := false
	// pip, pip3, and python3 -m pip name the same tool. A ready-installed pip
	// may answer through only one of them.
	for _, query := range [][]string{
		{"pip", "cache", "dir"},
		{"pip3", "cache", "dir"},
		{"python3", "-m", "pip", "cache", "dir"},
	} {
		if path, ok := toolQuery(ctx, query[0], query[1:]...); ok {
			cacheDir, answered = path, true
			break
		}
	}

	home, _ := os.UserHomeDir()
	candidatePaths := []string{
		cacheDir,
		filepath.Join(home, "Library", "Caches", "pip"),
		filepath.Join(home, ".cache", "pip"),
	}

	blocked := ""
	if !answered {
		// A guessed folder would move a cache that pip does not read.
		blocked = toolMissingReason("pip", "pip cache dir")
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
				ID:             "pip.cache",
				ToolName:       "pip",
				ToolVersion:    toolVersion(ctx, "pip"),
				Title:          "Pip Cache",
				Category:       core.CategoryPackageStore,
				CurrentPath:    path,
				ConfiguredPath: cacheDir,
				SizeBytes:      size,
				IsActive:       true,
				IsMigrated:     isMigrated,
				IsSymlink:      isSymlink,
				RedirectMethod: core.RedirectNativeConfig,
				TargetSubdir:   "pip-cache",
				BlockedReason:  blocked,
			})
			break
		}
	}

	return items, nil
}

func (p *PipProvider) PreflightCheck(ctx context.Context, item core.CacheItem, targetRoot string) error {
	running, err := fsutil.CheckRunningProcesses("pip", "pip3")
	if err != nil {
		return err
	}
	if len(running) > 0 {
		return fmt.Errorf("pip is running (PID: %s): stop active pip tasks before migration", running[0].PID)
	}
	return envOverride("PIP_CACHE_DIR")
}

func (p *PipProvider) Migrate(ctx context.Context, item core.CacheItem, targetRoot string, progress fsutil.MigrateProgress) error {
	dest := filepath.Join(targetRoot, item.TargetSubdir)
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}

	if err := fsutil.CopyDir(item.CurrentPath, dest, item.SizeBytes, progress.Copy); err != nil {
		return fmt.Errorf("copy pip cache: %w", err)
	}
	if err := fsutil.VerifyTreeProgress(item.CurrentPath, dest, progress.Verify); err != nil {
		return fmt.Errorf("pip cache: %w", err)
	}

	if err := core.RecordMigration(core.MigrationRecord{
		ID:             item.ID,
		ToolName:       p.Name(),
		OriginalPath:   item.CurrentPath,
		DestinationDir: dest,
		RedirectMethod: core.RedirectNativeConfig,
		OriginalConfig: item.ConfiguredPath,
		MigratedAt:     time.Now(),
		SizeBytes:      item.SizeBytes,
	}); err != nil {
		return err
	}

	if err := runCommand(ctx, "pip", "config", "set", "global.cache-dir", dest); err != nil {
		return switchFailed(item.ID, err)
	}

	return removeLocalSource(item.CurrentPath, dest)
}

func (p *PipProvider) Rollback(ctx context.Context, item core.CacheItem) error {
	rec, err := loadRecord(item.ID)
	if err != nil {
		return err
	}

	if rec.OriginalConfig != "" && rec.OriginalConfig != rec.DestinationDir {
		if err := runCommand(ctx, "pip", "config", "set", "global.cache-dir", rec.OriginalConfig); err != nil {
			return err
		}
	} else {
		if err := runCommand(ctx, "pip", "config", "unset", "global.cache-dir"); err != nil {
			return err
		}
	}

	if err := restoreNativeData(rec); err != nil {
		return err
	}

	return core.RemoveRecord(item.ID)
}
