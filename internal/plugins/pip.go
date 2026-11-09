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
	cmd := exec.CommandContext(ctx, "pip", "cache", "dir")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err == nil {
		cacheDir = strings.TrimSpace(out.String())
	}

	home, _ := os.UserHomeDir()
	candidatePaths := []string{
		cacheDir,
		filepath.Join(home, "Library", "Caches", "pip"),
		filepath.Join(home, ".cache", "pip"),
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
				ID:             "pip.cache",
				ToolName:       "pip",
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
			})
			break
		}
	}

	return items, nil
}

func (p *PipProvider) PreflightCheck(ctx context.Context, item core.CacheItem, targetRoot string) error {
	running := fsutil.CheckRunningProcesses("pip", "pip3")
	if len(running) > 0 {
		return fmt.Errorf("pip is actively running (PID: %s)", running[0].PID)
	}
	return nil
}

func (p *PipProvider) Migrate(ctx context.Context, item core.CacheItem, targetRoot string, onProgress fsutil.CopyProgressCallback) error {
	dest := filepath.Join(targetRoot, item.TargetSubdir)
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}

	if err := fsutil.CopyDir(item.CurrentPath, dest, item.SizeBytes, onProgress); err != nil {
		return fmt.Errorf("copy pip cache: %w", err)
	}

	cmd := exec.CommandContext(ctx, "pip", "config", "set", "global.cache-dir", dest)
	_ = cmd.Run()

	return core.RecordMigration(core.MigrationRecord{
		ID:             item.ID,
		ToolName:       p.Name(),
		OriginalPath:   item.CurrentPath,
		DestinationDir: dest,
		RedirectMethod: core.RedirectNativeConfig,
		OriginalConfig: item.ConfiguredPath,
		MigratedAt:     time.Now(),
		SizeBytes:      item.SizeBytes,
	})
}

func (p *PipProvider) Rollback(ctx context.Context, item core.CacheItem) error {
	m, err := core.LoadManifest()
	if err != nil {
		return err
	}
	rec, ok := m.Records[item.ID]
	if !ok {
		return fmt.Errorf("no migration record for %s", item.ID)
	}

	if rec.OriginalConfig != "" && rec.OriginalConfig != rec.DestinationDir {
		cmd := exec.CommandContext(ctx, "pip", "config", "set", "global.cache-dir", rec.OriginalConfig)
		_ = cmd.Run()
	} else {
		cmd := exec.CommandContext(ctx, "pip", "config", "unset", "global.cache-dir")
		_ = cmd.Run()
	}

	return core.RemoveRecord(item.ID)
}
