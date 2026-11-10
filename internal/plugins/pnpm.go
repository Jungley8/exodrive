package plugins

import (
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

	// pnpm reports the store of its own version: <store-dir>/v<N>. It appends
	// that folder to store-dir on every run. Exodrive moves the store root and
	// keeps the folder, so the new store-dir holds v<N> again.
	storePath := pnpmStorePath(ctx)
	storeRoot := storePath
	if isStoreVersion(filepath.Base(storePath)) {
		storeRoot = filepath.Dir(storePath)
	}

	if storePath != "" {
		if _, err := os.Stat(storePath); err != nil {
			// A ledger record means Exodrive moved this store. A missing path
			// then means the move went wrong, not that the store is empty.
			if m, mErr := core.LoadManifest(); mErr == nil {
				if _, recorded := m.Records["pnpm.store"]; recorded {
					return nil, fmt.Errorf("pnpm store is missing at %s: run 'exodrive rollback pnpm.store', then migrate again", storePath)
				}
			}
		}
	}

	home, _ := os.UserHomeDir()
	candidatePaths := []string{
		storeRoot,
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
			isActive := (path == storeRoot)

			// The store folder name carries the store layout version.
			var storeVersion string
			if isStoreVersion(filepath.Base(storePath)) {
				storeVersion = filepath.Base(storePath)
			}

			warning := ""
			if !isMigrated {
				warning = "Cross-disk notice: pnpm uses hardlinks. Keep store on the same volume as projects."
			}

			items = append(items, core.CacheItem{
				ID:             "pnpm.store",
				ToolName:       "pnpm",
				ToolVersion:    storeVersion,
				Title:          "PNPM Package Store",
				Category:       core.CategoryPackageStore,
				CurrentPath:    path,
				ConfiguredPath: pnpmStoreDir(ctx),
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

// pnpmStorePath returns the path pnpm uses for its store, empty when pnpm does
// not answer.
func pnpmStorePath(ctx context.Context) string {
	out, err := exec.CommandContext(ctx, "pnpm", "store", "path").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// pnpmStoreDir returns the value of the store-dir setting, empty when pnpm
// does not set it.
func pnpmStoreDir(ctx context.Context) string {
	out, err := exec.CommandContext(ctx, "pnpm", "config", "get", "store-dir").Output()
	if err != nil {
		return ""
	}
	value := strings.TrimSpace(string(out))
	if value == "undefined" {
		return ""
	}
	return value
}

// isStoreVersion reports whether name is a store version folder, such as v11.
func isStoreVersion(name string) bool {
	if len(name) < 2 || name[0] != 'v' {
		return false
	}
	for _, r := range name[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
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

func (p *PNPMProvider) Migrate(ctx context.Context, item core.CacheItem, targetRoot string, progress fsutil.MigrateProgress) error {
	dest := filepath.Join(targetRoot, item.TargetSubdir)
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}

	if err := fsutil.CopyDir(item.CurrentPath, dest, item.SizeBytes, progress.Copy); err != nil {
		return fmt.Errorf("copy pnpm store: %w", err)
	}
	if err := fsutil.VerifyTreeProgress(item.CurrentPath, dest, progress.Verify); err != nil {
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

	// store-dir points at the root. pnpm adds its own v<N> folder below it, and
	// the copy kept that folder, so pnpm finds the data again.
	if rec.OriginalConfig != "" {
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
