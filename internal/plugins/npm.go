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

type NPMProvider struct{}

func NewNPMProvider() *NPMProvider {
	return &NPMProvider{}
}

func (p *NPMProvider) Name() string {
	return "npm"
}

func (p *NPMProvider) Detect(ctx context.Context) ([]core.CacheItem, error) {
	var items []core.CacheItem

	// 1. Query npm CLI
	var configPath string
	cmd := exec.CommandContext(ctx, "npm", "config", "get", "cache")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err == nil {
		configPath = strings.TrimSpace(out.String())
	}

	home, _ := os.UserHomeDir()
	defaultPath := filepath.Join(home, ".npm")

	activePath := configPath
	if activePath == "" {
		activePath = defaultPath
	}

	if fi, err := os.Stat(activePath); err == nil && fi.IsDir() {
		size, _ := fsutil.DirSize(ctx, activePath)
		isSymlink := fsutil.IsSymlink(activePath)
		isMigrated := strings.HasPrefix(activePath, "/Volumes/") || isSymlink

		items = append(items, core.CacheItem{
			ID:             "npm.cache",
			ToolName:       "npm",
			ToolVersion:    toolVersion(ctx, "npm"),
			Title:          "NPM Global Cache",
			Category:       core.CategoryPackageStore,
			CurrentPath:    activePath,
			ConfiguredPath: configPath,
			SizeBytes:      size,
			IsActive:       true,
			IsMigrated:     isMigrated,
			IsSymlink:      isSymlink,
			RedirectMethod: core.RedirectNativeConfig,
			TargetSubdir:   "npm-cache",
		})
	}

	return items, nil
}

func (p *NPMProvider) PreflightCheck(ctx context.Context, item core.CacheItem, targetRoot string) error {
	running, err := fsutil.CheckRunningProcesses("npm", "npx")
	if err != nil {
		return err
	}
	if len(running) > 0 {
		return fmt.Errorf("npm is running (PID: %s): stop active npm processes before migration", running[0].PID)
	}
	return envOverride("npm_config_cache", "NPM_CONFIG_CACHE")
}

func (p *NPMProvider) Migrate(ctx context.Context, item core.CacheItem, targetRoot string, onProgress fsutil.CopyProgressCallback) error {
	dest := filepath.Join(targetRoot, item.TargetSubdir)
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}

	// Copy cache files
	if err := fsutil.CopyDir(item.CurrentPath, dest, item.SizeBytes, onProgress); err != nil {
		return fmt.Errorf("failed copying npm cache: %w", err)
	}
	if err := fsutil.VerifyTree(item.CurrentPath, dest); err != nil {
		return fmt.Errorf("npm cache: %w", err)
	}

	// Record the move before the switch. If the process stops between the two
	// steps, the record tells rollback where the data is.
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

	// Update npm global config
	cmd := exec.CommandContext(ctx, "npm", "config", "set", "cache", dest, "--global")
	if out, err := cmd.CombinedOutput(); err != nil {
		return switchFailed(item.ID, fmt.Errorf("npm config set cache failed (%s): %w", string(out), err))
	}

	return removeLocalSource(item.CurrentPath, dest)
}

func (p *NPMProvider) Rollback(ctx context.Context, item core.CacheItem) error {
	rec, err := loadRecord(item.ID)
	if err != nil {
		return err
	}

	// Reset npm config
	if rec.OriginalConfig != "" && rec.OriginalConfig != rec.DestinationDir {
		if err := runCommand(ctx, "npm", "config", "set", "cache", rec.OriginalConfig, "--global"); err != nil {
			return err
		}
	} else {
		if err := runCommand(ctx, "npm", "config", "delete", "cache", "--global"); err != nil {
			return err
		}
	}

	if err := restoreNativeData(rec); err != nil {
		return err
	}

	return core.RemoveRecord(item.ID)
}
