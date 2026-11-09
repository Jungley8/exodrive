package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"exodrive/internal/core"
	"exodrive/internal/fsutil"
)

type DockerProvider struct{}

func NewDockerProvider() *DockerProvider {
	return &DockerProvider{}
}

func (p *DockerProvider) Name() string {
	return "docker"
}

func (p *DockerProvider) Detect(ctx context.Context) ([]core.CacheItem, error) {
	var items []core.CacheItem
	home, _ := os.UserHomeDir()

	// 1. Detect OrbStack
	orbConfigPath := filepath.Join(home, ".orbstack", "config.json")
	var orbDataDir string
	if data, err := os.ReadFile(orbConfigPath); err == nil {
		var cfg struct {
			DataDir string `json:"data_dir"`
		}
		if json.Unmarshal(data, &cfg) == nil && cfg.DataDir != "" {
			orbDataDir = cfg.DataDir
		}
	}
	if orbDataDir == "" {
		orbDataDir = filepath.Join(home, "Library", "Application Support", "OrbStack", "data")
	}

	// Also check known user volume path if existing
	if fi, err := os.Stat("/Volumes/PHD/Docker/data"); err == nil && fi.IsDir() {
		orbDataDir = "/Volumes/PHD/Docker/data"
	}

	if _, err := os.Stat(orbDataDir); err == nil {
		size, _ := fsutil.DirSize(orbDataDir)
		isMigrated := strings.HasPrefix(orbDataDir, "/Volumes/")
		items = append(items, core.CacheItem{
			ID:             "docker.orbstack",
			ToolName:       "docker",
			Title:          "OrbStack VM Data & Images",
			Category:       core.CategoryVirtualDisk,
			CurrentPath:    orbDataDir,
			ConfiguredPath: orbDataDir,
			SizeBytes:      size,
			IsActive:       true,
			IsMigrated:     isMigrated,
			IsSymlink:      fsutil.IsSymlink(orbDataDir),
			RedirectMethod: core.RedirectNativeConfig,
			TargetSubdir:   filepath.Join("Docker", "orbstack"),
			Warning:        "Safety guard: Stop OrbStack before migration to prevent disk image corruption.",
		})
	}

	// 2. Detect Docker Desktop
	dockerRaw := filepath.Join(home, "Library", "Containers", "com.docker.docker", "Data", "vms", "0", "data", "Docker.raw")
	if fi, err := os.Stat(dockerRaw); err == nil && !fi.IsDir() {
		items = append(items, core.CacheItem{
			ID:             "docker.desktop",
			ToolName:       "docker",
			Title:          "Docker Desktop Virtual Disk (Docker.raw)",
			Category:       core.CategoryVirtualDisk,
			CurrentPath:    dockerRaw,
			SizeBytes:      fi.Size(),
			IsActive:       true,
			IsMigrated:     strings.HasPrefix(dockerRaw, "/Volumes/") || fsutil.IsSymlink(dockerRaw),
			IsSymlink:      fsutil.IsSymlink(dockerRaw),
			RedirectMethod: core.RedirectSymlink,
			TargetSubdir:   filepath.Join("Docker", "docker-desktop"),
			Warning:        "Safety guard: Quit Docker Desktop completely before migrating Docker.raw.",
		})
	}

	// 3. Detect Colima
	colimaDir := filepath.Join(home, ".colima")
	if fi, err := os.Stat(colimaDir); err == nil && fi.IsDir() {
		size, _ := fsutil.DirSize(colimaDir)
		items = append(items, core.CacheItem{
			ID:             "docker.colima",
			ToolName:       "docker",
			Title:          "Colima VM Disks",
			Category:       core.CategoryVirtualDisk,
			CurrentPath:    colimaDir,
			SizeBytes:      size,
			IsActive:       true,
			IsMigrated:     strings.HasPrefix(colimaDir, "/Volumes/") || fsutil.IsSymlink(colimaDir),
			IsSymlink:      fsutil.IsSymlink(colimaDir),
			RedirectMethod: core.RedirectSymlink,
			TargetSubdir:   filepath.Join("Docker", "colima"),
			Warning:        "Safety guard: Run 'colima stop' before migrating VM disks.",
		})
	}

	return items, nil
}

func (p *DockerProvider) PreflightCheck(ctx context.Context, item core.CacheItem, targetRoot string) error {
	running := fsutil.CheckRunningProcesses("Docker", "com.docker.backend", "OrbStack", "orbstack", "colima")
	if len(running) > 0 {
		return fmt.Errorf("container engine is actively running (%s, PID: %s). Please quit/stop it before migrating VM images", running[0].Name, running[0].PID)
	}
	return nil
}

func (p *DockerProvider) Migrate(ctx context.Context, item core.CacheItem, targetRoot string, onProgress fsutil.CopyProgressCallback) error {
	dest := filepath.Join(targetRoot, item.TargetSubdir)
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}

	if item.ID == "docker.orbstack" {
		// Try using orb CLI if available
		cmd := exec.CommandContext(ctx, "orb", "config", "set", "data_dir", dest)
		var out bytes.Buffer
		cmd.Stdout = &out
		if err := cmd.Run(); err == nil {
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
	}

	// Copy and symlink fallback
	if !item.IsSymlink {
		if err := fsutil.CopyDir(item.CurrentPath, dest, item.SizeBytes, onProgress); err != nil {
			return fmt.Errorf("copy %s: %w", item.Title, err)
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

func (p *DockerProvider) Rollback(ctx context.Context, item core.CacheItem) error {
	m, err := core.LoadManifest()
	if err != nil {
		return err
	}
	rec, ok := m.Records[item.ID]
	if !ok {
		return fmt.Errorf("no migration record for %s", item.ID)
	}

	if item.ID == "docker.orbstack" && rec.OriginalConfig != "" {
		cmd := exec.CommandContext(ctx, "orb", "config", "set", "data_dir", rec.OriginalConfig)
		_ = cmd.Run()
	}

	backupDir := rec.OriginalPath + ".bak"
	if _, err := os.Stat(backupDir); err == nil {
		_ = os.Remove(rec.OriginalPath)
		_ = os.Rename(backupDir, rec.OriginalPath)
	}

	return core.RemoveRecord(item.ID)
}
