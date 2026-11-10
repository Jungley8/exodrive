package plugins

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"exodrive/internal/core"
	"exodrive/internal/fsutil"
)

type YarnProvider struct{}

func NewYarnProvider() *YarnProvider {
	return &YarnProvider{}
}

func (p *YarnProvider) Name() string {
	return "yarn"
}

func (p *YarnProvider) Detect(ctx context.Context) ([]core.CacheItem, error) {
	var items []core.CacheItem

	var cacheDir string
	cmd := exec.CommandContext(ctx, "yarn", "cache", "dir")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err == nil {
		cacheDir = strings.TrimSpace(out.String())
	}

	home, _ := os.UserHomeDir()
	candidatePaths := []string{
		cacheDir,
		filepath.Join(home, "Library", "Caches", "Yarn"),
		filepath.Join(home, ".cache", "yarn"),
		filepath.Join(home, ".yarn", "berry", "cache"),
	}

	seen := make(map[string]bool)
	for _, path := range candidatePaths {
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true

		if fi, err := os.Stat(path); err == nil && fi.IsDir() {
			size, _ := fsutil.DirSize(path)
			isSymlink := fsutil.IsSymlink(path)
			isMigrated := strings.HasPrefix(path, "/Volumes/") || isSymlink

			items = append(items, core.CacheItem{
				ID:             "yarn.cache",
				ToolName:       "yarn",
				Title:          "Yarn Cache",
				Category:       core.CategoryPackageStore,
				CurrentPath:    path,
				ConfiguredPath: cacheDir,
				SizeBytes:      size,
				IsActive:       true,
				IsMigrated:     isMigrated,
				IsSymlink:      isSymlink,
				RedirectMethod: core.RedirectNativeConfig,
				TargetSubdir:   "yarn-cache",
			})
			break
		}
	}

	return items, nil
}

func (p *YarnProvider) PreflightCheck(ctx context.Context, item core.CacheItem, targetRoot string) error {
	running := fsutil.CheckRunningProcesses("yarn")
	if len(running) > 0 {
		return fmt.Errorf("yarn is running (PID: %s): stop active yarn tasks before migration", running[0].PID)
	}
	return envOverride("YARN_CACHE_FOLDER")
}

func (p *YarnProvider) Migrate(ctx context.Context, item core.CacheItem, targetRoot string, onProgress fsutil.CopyProgressCallback) error {
	dest := filepath.Join(targetRoot, item.TargetSubdir)
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}

	if err := fsutil.CopyDir(item.CurrentPath, dest, item.SizeBytes, onProgress); err != nil {
		return fmt.Errorf("copy yarn cache: %w", err)
	}
	if err := fsutil.VerifyTree(item.CurrentPath, dest); err != nil {
		return fmt.Errorf("yarn cache: %w", err)
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

	// Yarn writes a project .yarnrc without --global, which would leave the
	// global cache unchanged.
	if err := runCommand(ctx, "yarn", "config", "set", "cache-folder", dest, "--global"); err != nil {
		return switchFailed(item.ID, err)
	}

	return removeLocalSource(item.CurrentPath, dest)
}

func (p *YarnProvider) Rollback(ctx context.Context, item core.CacheItem) error {
	rec, err := loadRecord(item.ID)
	if err != nil {
		return err
	}

	if rec.OriginalConfig != "" && rec.OriginalConfig != rec.DestinationDir {
		if err := runCommand(ctx, "yarn", "config", "set", "cache-folder", rec.OriginalConfig, "--global"); err != nil {
			return err
		}
	} else {
		if err := runCommand(ctx, "yarn", "config", "delete", "cache-folder", "--global"); err != nil {
			return err
		}
	}

	if err := restoreNativeData(rec); err != nil {
		return err
	}

	return core.RemoveRecord(item.ID)
}
