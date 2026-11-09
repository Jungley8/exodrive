package core

import (
	"context"
	"fmt"
	"os"
	"sync"

	"exodrive/internal/fsutil"
)

// ProviderAccessor is an interface to avoid circular dependency with plugins package.
type ProviderAccessor interface {
	Name() string
	Detect(ctx context.Context) ([]CacheItem, error)
	PreflightCheck(ctx context.Context, item CacheItem, targetRoot string) error
	Migrate(ctx context.Context, item CacheItem, targetRoot string, onProgress fsutil.CopyProgressCallback) error
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

// ScanAll scans across all registered providers in parallel.
func (e *Engine) ScanAll(ctx context.Context) ([]CacheItem, error) {
	var mu sync.Mutex
	var allItems []CacheItem

	var wg sync.WaitGroup
	for _, p := range e.providers {
		wg.Add(1)
		go func(prov ProviderAccessor) {
			defer wg.Done()
			items, err := prov.Detect(ctx)
			if err == nil && len(items) > 0 {
				mu.Lock()
				allItems = append(allItems, items...)
				mu.Unlock()
			}
		}(p)
	}

	wg.Wait()
	return allItems, nil
}

// Preflight evaluates whether targetRoot is valid and has sufficient space.
func (e *Engine) Preflight(ctx context.Context, item CacheItem, targetRoot string) error {
	freeBytes, _, fstype, err := fsutil.GetDiskUsage(targetRoot)
	if err != nil {
		return fmt.Errorf("target volume inaccessible: %w", err)
	}

	if !fsutil.IsSafeFilesystem(fstype) {
		return fmt.Errorf("incompatible filesystem '%s': dev caches require APFS/HFS+ on macOS or ext4/btrfs on Linux. exFAT/FAT32 is forbidden", fstype)
	}

	required := int64(float64(item.SizeBytes) * 1.1) // 10% headroom
	if freeBytes < required {
		return fmt.Errorf("insufficient space on destination: need %d MB, only %d MB available", required/1024/1024, freeBytes/1024/1024)
	}

	for _, p := range e.providers {
		if p.Name() == item.ToolName {
			return p.PreflightCheck(ctx, item, targetRoot)
		}
	}
	return nil
}

// Migrate executes migration for the item matching itemID.
func (e *Engine) Migrate(ctx context.Context, item CacheItem, targetRoot string, onProgress fsutil.CopyProgressCallback) error {
	if err := e.Preflight(ctx, item, targetRoot); err != nil {
		return fmt.Errorf("preflight failed: %w", err)
	}

	for _, p := range e.providers {
		if p.Name() == item.ToolName {
			return p.Migrate(ctx, item, targetRoot, onProgress)
		}
	}
	return fmt.Errorf("no provider found for %s", item.ToolName)
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

// CleanOrphan deletes an abandoned cache directory.
func (e *Engine) CleanOrphan(path string) error {
	return os.RemoveAll(path)
}
