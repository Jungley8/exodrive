package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"exodrive/internal/fsutil"
)

// ErrLedger marks a scan that could not read the migration ledger. A caller
// must stop: without the ledger, Exodrive cannot tell a migrated cache from a
// local one.
var ErrLedger = errors.New("read migration ledger")

// ProviderAccessor is an interface to avoid circular dependency with plugins package.
type ProviderAccessor interface {
	Name() string
	Detect(ctx context.Context) ([]CacheItem, error)
	PreflightCheck(ctx context.Context, item CacheItem, targetRoot string) error
	Migrate(ctx context.Context, item CacheItem, targetRoot string, progress fsutil.MigrateProgress) error
	Rollback(ctx context.Context, item CacheItem) error
}

type Engine struct {
	providers []ProviderAccessor
}

func NewEngine(providers ...ProviderAccessor) *Engine {
	return &Engine{
		providers: providers,
	}
}

// ScanAll scans across all registered providers in parallel. It marks an item
// as migrated when the ledger holds a record for it, so a migration is never
// repeated for an item the user already moved. A provider that detects no items
// reports nothing. A provider that fails returns an error beside the items that
// the other providers found.
func (e *Engine) ScanAll(ctx context.Context) ([]CacheItem, error) {
	var mu sync.Mutex
	var allItems []CacheItem
	var detectErrs []error

	var wg sync.WaitGroup
	for _, p := range e.providers {
		wg.Add(1)
		go func(prov ProviderAccessor) {
			defer wg.Done()
			items, err := prov.Detect(ctx)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				detectErrs = append(detectErrs, fmt.Errorf("%s: %w", prov.Name(), err))
			}
			allItems = append(allItems, items...)
		}(p)
	}

	wg.Wait()

	manifest, err := LoadManifest()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLedger, err)
	}
	for i := range allItems {
		if _, ok := manifest.Records[allItems[i].ID]; ok {
			allItems[i].IsMigrated = true
		}
	}
	return allItems, errors.Join(detectErrs...)
}

// provider returns the registered provider for an item. It fails closed: an
// item whose toolchain has no provider is an error, never a silent pass.
func (e *Engine) provider(item CacheItem) (ProviderAccessor, error) {
	for _, p := range e.providers {
		if p.Name() == item.ToolName {
			return p, nil
		}
	}
	return nil, fmt.Errorf("no provider registered for toolchain %q (item %s)", item.ToolName, item.ID)
}

// Preflight evaluates whether targetRoot is valid and has sufficient space.
// TargetInfo describes the volume that holds a target folder.
type TargetInfo struct {
	Path       string `json:"path"`
	Filesystem string `json:"filesystem"`
	FreeBytes  int64  `json:"free_bytes"`
	TotalBytes int64  `json:"total_bytes"`
}

// CheckTarget reports the volume under targetRoot before anything moves. It
// refuses a folder it cannot read and a filesystem outside the allowlist, so
// the window and the command line show the same reason.
func (e *Engine) CheckTarget(targetRoot string) (TargetInfo, error) {
	if info, err := os.Stat(targetRoot); err != nil || !info.IsDir() {
		return TargetInfo{}, targetMissingError(targetRoot)
	}
	freeBytes, totalBytes, fstype, err := fsutil.GetDiskUsage(targetRoot)
	if err != nil {
		return TargetInfo{}, fmt.Errorf("target folder is not available: %w", err)
	}
	if !fsutil.IsSafeFilesystem(fstype) {
		return TargetInfo{}, fmt.Errorf("filesystem '%s' is not supported: choose APFS, HFS+, ext2/3/4, btrfs, zfs, xfs, f2fs, or bcachefs", fstype)
	}
	return TargetInfo{Path: targetRoot, Filesystem: fstype, FreeBytes: freeBytes, TotalBytes: totalBytes}, nil
}

func (e *Engine) Preflight(ctx context.Context, item CacheItem, targetRoot string) error {
	if item.BlockedReason != "" {
		return fmt.Errorf("%s: %s", item.Title, item.BlockedReason)
	}

	p, err := e.provider(item)
	if err != nil {
		return err
	}

	if info, statErr := os.Stat(targetRoot); statErr != nil || !info.IsDir() {
		return targetMissingError(targetRoot)
	}

	freeBytes, _, fstype, err := fsutil.GetDiskUsage(targetRoot)
	if err != nil {
		return fmt.Errorf("target volume inaccessible: %w", err)
	}

	if !fsutil.IsSafeFilesystem(fstype) {
		return fmt.Errorf("incompatible filesystem '%s': target storage requires APFS, HFS+, ext2/3/4, btrfs, zfs, xfs, f2fs, or bcachefs", fstype)
	}

	required := int64(float64(item.SizeBytes) * 1.1) // 10% headroom
	if freeBytes < required {
		return fmt.Errorf("insufficient space on destination: need %d MB, only %d MB available", required/1024/1024, freeBytes/1024/1024)
	}

	return p.PreflightCheck(ctx, item, targetRoot)
}

// Migrate executes migration for the item matching itemID.
func (e *Engine) Migrate(ctx context.Context, item CacheItem, targetRoot string, progress fsutil.MigrateProgress) error {
	p, err := e.provider(item)
	if err != nil {
		return err
	}

	manifest, err := LoadManifest()
	if err != nil {
		return fmt.Errorf("read migration ledger: %w", err)
	}
	if rec, ok := manifest.Records[item.ID]; ok {
		return fmt.Errorf("%s is already migrated to %s (run 'exodrive rollback %s' first)", item.ID, rec.DestinationDir, item.ID)
	}

	if err := e.Preflight(ctx, item, targetRoot); err != nil {
		return fmt.Errorf("preflight failed: %w", err)
	}

	return p.Migrate(ctx, item, targetRoot, progress)
}

// Rollback restores a previously migrated item using its manifest record.
func (e *Engine) Rollback(ctx context.Context, itemID string) error {
	m, err := LoadManifest()
	if err != nil {
		return err
	}
	rec, ok := m.Records[itemID]
	if !ok {
		return fmt.Errorf("no record found for %s in manifest", itemID)
	}

	for _, p := range e.providers {
		if p.Name() == rec.ToolName {
			dummy := CacheItem{
				ID:             rec.ID,
				ToolName:       rec.ToolName,
				CurrentPath:    rec.OriginalPath,
				ConfiguredPath: rec.OriginalConfig,
				SizeBytes:      rec.SizeBytes,
			}
			return p.Rollback(ctx, dummy)
		}
	}
	return fmt.Errorf("no provider found for %s", rec.ToolName)
}

// CleanOrphan deletes an abandoned cache directory after strict path validation.
func (e *Engine) CleanOrphan(path string) error {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return fmt.Errorf("clean orphan: path cannot be empty")
	}

	cleanPath := filepath.Clean(trimmed)
	if !filepath.IsAbs(cleanPath) {
		return fmt.Errorf("clean orphan: path must be absolute: %q", path)
	}

	// Refuse root filesystem
	if cleanPath == "/" || cleanPath == filepath.VolumeName(cleanPath)+string(filepath.Separator) {
		return fmt.Errorf("clean orphan: refusing to delete root filesystem: %q", path)
	}

	// Refuse user home directory and anything that holds more than a cache.
	home, err := os.UserHomeDir()
	if err == nil {
		home = filepath.Clean(home)
		if cleanPath == home {
			return fmt.Errorf("clean orphan: refusing to delete user home directory: %q", path)
		}
		if strings.HasPrefix(cleanPath, home+string(filepath.Separator)) {
			relHome, relErr := filepath.Rel(home, cleanPath)
			if relErr != nil || len(strings.Split(relHome, string(filepath.Separator))) < 2 {
				return fmt.Errorf("clean orphan: refusing to delete top-level home directory: %q", path)
			}
		}
	}

	// Guard against shallow/top-level directories (e.g. /Users, /var, /etc, /tmp, /Volumes/disk).
	rel, err := filepath.Rel("/", cleanPath)
	if err != nil || rel == "." {
		return fmt.Errorf("clean orphan: invalid path hierarchy: %q", path)
	}
	segments := strings.Split(rel, string(filepath.Separator))
	if len(segments) < 3 {
		return fmt.Errorf("clean orphan: refusing to delete shallow root directory: %q", path)
	}

	return os.RemoveAll(cleanPath)
}
