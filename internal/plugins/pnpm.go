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

type PNPMProvider struct{}

func NewPNPMProvider() *PNPMProvider {
	return &PNPMProvider{}
}

func (p *PNPMProvider) Name() string {
	return "pnpm"
}

func (p *PNPMProvider) Detect(ctx context.Context) ([]core.CacheItem, error) {
	var items []core.CacheItem

	var storePath string
	cmd := exec.CommandContext(ctx, "pnpm", "store", "path")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err == nil {
		storePath = strings.TrimSpace(out.String())
	}

	home, _ := os.UserHomeDir()
	candidatePaths := []string{
		storePath,
		filepath.Join(home, "Library", "pnpm", "store"),
		filepath.Join(home, ".pnpm-store"),
		filepath.Join(home, "Library", "Caches", "pnpm"),
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
			isActive := (path == storePath)

			warning := ""
			if !isMigrated {
				warning = "Cross-disk notice: pnpm uses hardlinks. Keep store on the same volume as projects."
			}

			items = append(items, core.CacheItem{
				ID:             "pnpm.store",
				ToolName:       "pnpm",
				Title:          "PNPM Package Store",
				Category:       core.CategoryPackageStore,
				CurrentPath:    path,
				ConfiguredPath: storePath,
				SizeBytes:      size,
				IsActive:       isActive,
				IsMigrated:     isMigrated,
				IsSymlink:      isSymlink,
				RedirectMethod: core.RedirectNativeConfig,
				TargetSubdir:   "pnpm-store",
				Warning:        warning,
			})
			break
		}
	}

	return items, nil
}

func (p *PNPMProvider) PreflightCheck(ctx context.Context, item core.CacheItem, targetRoot string) error {
	running, err := fsutil.CheckRunningProcesses("pnpm")
	if err != nil {
		return err
	}
	if len(running) > 0 {
		return fmt.Errorf("pnpm is running (PID: %s): stop active pnpm tasks before migration", running[0].PID)
	}
	return envOverride("npm_config_store_dir", "NPM_CONFIG_STORE_DIR")
}

func (p *PNPMProvider) Migrate(ctx context.Context, item core.CacheItem, targetRoot string, onProgress fsutil.CopyProgressCallback) error {
	dest := filepath.Join(targetRoot, item.TargetSubdir)
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}

	if err := fsutil.CopyDir(item.CurrentPath, dest, item.SizeBytes, onProgress); err != nil {
		return fmt.Errorf("copy pnpm store: %w", err)
	}
	if err := fsutil.VerifyTree(item.CurrentPath, dest); err != nil {
		return fmt.Errorf("pnpm store: %w", err)
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

	cmd := exec.CommandContext(ctx, "pnpm", "config", "set", "store-dir", dest, "--global")
	if out, err := cmd.CombinedOutput(); err != nil {
		return switchFailed(item.ID, fmt.Errorf("pnpm config set store-dir failed (%s): %w", string(out), err))
	}

	return removeLocalSource(item.CurrentPath, dest)
}

func (p *PNPMProvider) Rollback(ctx context.Context, item core.CacheItem) error {
	rec, err := loadRecord(item.ID)
	if err != nil {
		return err
	}

	if rec.OriginalConfig != "" && rec.OriginalConfig != rec.DestinationDir {
		if err := runCommand(ctx, "pnpm", "config", "set", "store-dir", rec.OriginalConfig, "--global"); err != nil {
			return err
		}
	} else {
		if err := runCommand(ctx, "pnpm", "config", "delete", "store-dir", "--global"); err != nil {
			return err
		}
	}

	if err := restoreNativeData(rec); err != nil {
		return err
	}

	return core.RemoveRecord(item.ID)
}
