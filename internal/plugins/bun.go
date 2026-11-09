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

type BunProvider struct{}

func NewBunProvider() *BunProvider {
	return &BunProvider{}
}

func (p *BunProvider) Name() string {
	return "bun"
}

func (p *BunProvider) Detect(ctx context.Context) ([]core.CacheItem, error) {
	var items []core.CacheItem

	var cacheDir string
	cmd := exec.CommandContext(ctx, "bun", "pm", "cache")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err == nil {
		cacheDir = strings.TrimSpace(out.String())
	}

	home, _ := os.UserHomeDir()
	candidatePaths := []string{
		cacheDir,
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
			size, _ := fsutil.DirSize(path)
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
				Warning:        "Safety rule: ~/.bun/bin is preserved locally on internal drive.",
			})
			break
		}
	}

	return items, nil
}

func (p *BunProvider) PreflightCheck(ctx context.Context, item core.CacheItem, targetRoot string) error {
	running := fsutil.CheckRunningProcesses("bun")
	if len(running) > 0 {
		return fmt.Errorf("bun is actively running (PID: %s)", running[0].PID)
	}
	return nil
}

func (p *BunProvider) Migrate(ctx context.Context, item core.CacheItem, targetRoot string, onProgress fsutil.CopyProgressCallback) error {
	dest := filepath.Join(targetRoot, item.TargetSubdir)
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}

	if !item.IsSymlink {
		if err := fsutil.CopyDir(item.CurrentPath, dest, item.SizeBytes, onProgress); err != nil {
			return fmt.Errorf("copy bun cache: %w", err)
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

func (p *BunProvider) Rollback(ctx context.Context, item core.CacheItem) error {
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
		_ = os.Remove(rec.OriginalPath)
		_ = os.Rename(backupDir, rec.OriginalPath)
	}

	return core.RemoveRecord(item.ID)
}
