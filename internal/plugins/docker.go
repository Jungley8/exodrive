package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
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

// orbStackConfigPath returns the file that holds OrbStack's data_dir setting.
func orbStackConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, ".orbstack", "config.json"), nil
}

// readOrbStackConfig reads OrbStack's configuration, keeping every key it holds.
func readOrbStackConfig(path string) (map[string]any, error) {
	cfg := make(map[string]any)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

// writeOrbStackConfig replaces OrbStack's configuration atomically.
func writeOrbStackConfig(path string, cfg map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// dataDir returns the configured data_dir, empty when OrbStack does not set one.
func orbDataDir(cfg map[string]any) string {
	dir, _ := cfg["data_dir"].(string)
	return strings.TrimSpace(dir)
}

func (p *DockerProvider) Detect(ctx context.Context) ([]core.CacheItem, error) {
	var items []core.CacheItem
	home, _ := os.UserHomeDir()

	// 1. Detect OrbStack
	cfgPath, err := orbStackConfigPath()
	if err != nil {
		return nil, err
	}
	cfg, err := readOrbStackConfig(cfgPath)
	if err != nil {
		return nil, err
	}
	orbDataDirPath := orbDataDir(cfg)
	if orbDataDirPath == "" {
		orbDataDirPath = filepath.Join(home, "Library", "Application Support", "OrbStack", "data")
	}

	if envDir := os.Getenv("ORBSTACK_DATA_DIR"); envDir != "" {
		if fi, err := os.Stat(envDir); err == nil && fi.IsDir() {
			orbDataDirPath = envDir
		}
	}

	if _, err := os.Stat(orbDataDirPath); err == nil {
		size, _ := fsutil.DirSize(ctx, orbDataDirPath)
		items = append(items, core.CacheItem{
			ID:             "docker.orbstack",
			ToolName:       "docker",
			Title:          "OrbStack VM Data & Images",
			Category:       core.CategoryVirtualDisk,
			CurrentPath:    orbDataDirPath,
			ConfiguredPath: orbDataDir(cfg),
			SizeBytes:      size,
			IsActive:       true,
			IsMigrated:     fsutil.IsSymlink(orbDataDirPath),
			IsSymlink:      fsutil.IsSymlink(orbDataDirPath),
			RedirectMethod: core.RedirectNativeConfig,
			TargetSubdir:   filepath.Join("Docker", "orbstack"),
			Warning:        "Stop OrbStack before migration to prevent disk image corruption.",
		})
	}

	// 2. Detect Docker Desktop. Version 4.15 and later hold Docker.raw. Earlier
	// versions hold Docker.qcow2, so both names count.
	vmDataDir := filepath.Join(home, "Library", "Containers", "com.docker.docker", "Data", "vms", "0", "data")
	for _, diskName := range []string{"Docker.raw", "Docker.qcow2"} {
		diskPath := filepath.Join(vmDataDir, diskName)
		fi, statErr := os.Stat(diskPath)
		if statErr != nil || fi.IsDir() {
			continue
		}
		items = append(items, core.CacheItem{
			ID:             "docker.desktop",
			ToolName:       "docker",
			Title:          fmt.Sprintf("Docker Desktop Virtual Disk (%s)", diskName),
			Category:       core.CategoryVirtualDisk,
			CurrentPath:    diskPath,
			SizeBytes:      fi.Size(),
			IsActive:       true,
			IsMigrated:     fsutil.IsSymlink(diskPath),
			IsSymlink:      fsutil.IsSymlink(diskPath),
			RedirectMethod: core.RedirectSymlink,
			TargetSubdir:   filepath.Join("Docker", "docker-desktop"),
			Warning:        fmt.Sprintf("Stop Docker Desktop before migrating %s.", diskName),
		})
		break
	}

	// 3. Detect Colima
	colimaDir := filepath.Join(home, ".colima")
	if fi, err := os.Stat(colimaDir); err == nil && fi.IsDir() {
		size, _ := fsutil.DirSize(ctx, colimaDir)
		items = append(items, core.CacheItem{
			ID:             "docker.colima",
			ToolName:       "docker",
			Title:          "Colima VM Disks",
			Category:       core.CategoryVirtualDisk,
			CurrentPath:    colimaDir,
			SizeBytes:      size,
			IsActive:       true,
			IsMigrated:     fsutil.IsSymlink(colimaDir),
			IsSymlink:      fsutil.IsSymlink(colimaDir),
			RedirectMethod: core.RedirectSymlink,
			TargetSubdir:   filepath.Join("Docker", "colima"),
			Warning:        "Run 'colima stop' before migrating VM disks.",
		})
	}

	return items, nil
}

func (p *DockerProvider) PreflightCheck(ctx context.Context, item core.CacheItem, targetRoot string) error {
	running, err := fsutil.CheckRunningProcesses("Docker", "com.docker.backend", "OrbStack", "orbstack", "colima")
	if err != nil {
		return err
	}
	if len(running) > 0 {
		return fmt.Errorf("container engine is running (%s, PID: %s): stop the container engine before migration", running[0].Name, running[0].PID)
	}
	if item.ID == "docker.orbstack" {
		// The variable wins over data_dir in the config file, so the switch
		// would not take effect.
		return envOverride("ORBSTACK_DATA_DIR")
	}
	return nil
}

func (p *DockerProvider) Migrate(ctx context.Context, item core.CacheItem, targetRoot string, progress fsutil.MigrateProgress) error {
	dest := filepath.Join(targetRoot, item.TargetSubdir)
	if item.ID == "docker.orbstack" {
		if err := os.MkdirAll(dest, 0755); err != nil {
			return err
		}
		return p.migrateOrbStack(ctx, item, dest, progress)
	}
	return moveAndLink(p.Name(), item, dest, progress)
}

// migrateOrbStack copies the data first and points OrbStack at the copy only
// after the copy verifies, so the engine never starts on an empty directory.
func (p *DockerProvider) migrateOrbStack(ctx context.Context, item core.CacheItem, dest string, progress fsutil.MigrateProgress) error {
	if fsutil.IsSymlink(item.CurrentPath) {
		return fmt.Errorf("%s is already a symlink: nothing to migrate", item.CurrentPath)
	}

	if err := fsutil.CopyDir(item.CurrentPath, dest, item.SizeBytes, progress.Copy); err != nil {
		return fmt.Errorf("copy %s: %w", item.Title, err)
	}
	summary, err := fsutil.VerifyTreeSummary(item.CurrentPath, dest, progress.Verify)
	if err != nil {
		return fmt.Errorf("%s: %w", item.Title, err)
	}

	cfgPath, err := orbStackConfigPath()
	if err != nil {
		return err
	}
	cfg, err := readOrbStackConfig(cfgPath)
	if err != nil {
		return err
	}
	originalDir := orbDataDir(cfg)

	if err := core.RecordMigration(core.MigrationRecord{
		ID:             item.ID,
		ToolName:       p.Name(),
		OriginalPath:   item.CurrentPath,
		DestinationDir: dest,
		RedirectMethod: core.RedirectNativeConfig,
		OriginalConfig: originalDir,
		MigratedAt:     time.Now(),
		SizeBytes:      item.SizeBytes,
		TreeDigest:     string(summary.Digest),
		FileCount:      summary.Files,
		TotalBytes:     summary.Bytes,
	}); err != nil {
		return err
	}

	cfg["data_dir"] = dest
	if err := writeOrbStackConfig(cfgPath, cfg); err != nil {
		return switchFailed(item.ID, fmt.Errorf("set data_dir in %s: %w", cfgPath, err))
	}

	return removeLocalSource(item.CurrentPath, dest)
}

func (p *DockerProvider) Rollback(ctx context.Context, item core.CacheItem) error {
	rec, err := loadRecord(item.ID)
	if err != nil {
		return err
	}

	switch rec.RedirectMethod {
	case core.RedirectNativeConfig:
		return rollbackOrbStack(rec)
	case core.RedirectSymlink:
		if err := restoreSymlink(rec.OriginalPath, rec.DestinationDir); err != nil {
			return err
		}
		return core.RemoveRecord(item.ID)
	default:
		return fmt.Errorf("rollback: unsupported redirect method %q for %s", rec.RedirectMethod, item.ID)
	}
}

// rollbackOrbStack copies the data back, then points OrbStack at it, then
// removes the external copy. OrbStack always reads an existing directory.
// A local directory that already matches the external copy comes from an
// interrupted rollback, so Exodrive keeps it and skips the copy.
func rollbackOrbStack(rec core.MigrationRecord) error {
	if _, err := os.Stat(rec.DestinationDir); err != nil {
		return fmt.Errorf("rollback: destination %s is unavailable: %w", rec.DestinationDir, err)
	}

	switch _, err := os.Stat(rec.OriginalPath); {
	case err == nil:
		if err := fsutil.VerifyTree(rec.DestinationDir, rec.OriginalPath); err != nil {
			return fmt.Errorf("rollback: %s exists and differs from %s: %w", rec.OriginalPath, rec.DestinationDir, err)
		}
	case os.IsNotExist(err):
		if err := fsutil.CopyDir(rec.DestinationDir, rec.OriginalPath, 0, nil); err != nil {
			return fmt.Errorf("rollback: copy %s back: %w", rec.DestinationDir, err)
		}
		if err := fsutil.VerifyTree(rec.DestinationDir, rec.OriginalPath); err != nil {
			return fmt.Errorf("rollback: %w", err)
		}
	default:
		return fmt.Errorf("rollback: inspect %s: %w", rec.OriginalPath, err)
	}

	cfgPath, err := orbStackConfigPath()
	if err != nil {
		return err
	}
	cfg, err := readOrbStackConfig(cfgPath)
	if err != nil {
		return err
	}
	if rec.OriginalConfig != "" {
		cfg["data_dir"] = rec.OriginalConfig
	} else {
		delete(cfg, "data_dir")
	}
	if err := writeOrbStackConfig(cfgPath, cfg); err != nil {
		return fmt.Errorf("restore data_dir in %s: %w", cfgPath, err)
	}

	if err := os.RemoveAll(rec.DestinationDir); err != nil {
		return fmt.Errorf("rollback: remove %s: %w", rec.DestinationDir, err)
	}
	return core.RemoveRecord(rec.ID)
}
