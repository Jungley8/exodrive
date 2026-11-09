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

type UVProvider struct{}

func NewUVProvider() *UVProvider {
	return &UVProvider{}
}

func (p *UVProvider) Name() string {
	return "uv"
}

func (p *UVProvider) Detect(ctx context.Context) ([]core.CacheItem, error) {
	var items []core.CacheItem

	var cacheDir string
	cmd := exec.CommandContext(ctx, "uv", "cache", "dir")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err == nil {
		cacheDir = strings.TrimSpace(out.String())
	}

	home, _ := os.UserHomeDir()
	candidatePaths := []string{
		cacheDir,
		filepath.Join(home, "Library", "Caches", "uv"),
		filepath.Join(home, ".cache", "uv"),
	}

	for _, path := range candidatePaths {
		if path == "" {
			continue
		}
		if fi, err := os.Stat(path); err == nil && fi.IsDir() {
			size, _ := fsutil.DirSize(path)
			isSymlink := fsutil.IsSymlink(path)
			isMigrated := strings.HasPrefix(path, "/Volumes/") || isSymlink

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
			})
			break
		}
	}

	// Also detect UV Python toolchain installations
	pythonInstallDir := filepath.Join(home, ".local", "share", "uv", "python")
	if fi, err := os.Stat(pythonInstallDir); err == nil && fi.IsDir() {
		size, _ := fsutil.DirSize(pythonInstallDir)
		isSymlink := fsutil.IsSymlink(pythonInstallDir)
		isMigrated := strings.HasPrefix(pythonInstallDir, "/Volumes/") || isSymlink

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
	running := fsutil.CheckRunningProcesses("uv")
	if len(running) > 0 {
		return fmt.Errorf("uv is running (PID: %s): stop active uv tasks before migration", running[0].PID)
	}
	return nil
}

func (p *UVProvider) Migrate(ctx context.Context, item core.CacheItem, targetRoot string, onProgress fsutil.CopyProgressCallback) error {
	dest := filepath.Join(targetRoot, item.TargetSubdir)
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}

	if !item.IsSymlink {
		if err := fsutil.CopyDir(item.CurrentPath, dest, item.SizeBytes, onProgress); err != nil {
			return fmt.Errorf("copy uv %s: %w", item.Title, err)
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

func (p *UVProvider) Rollback(ctx context.Context, item core.CacheItem) error {
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
