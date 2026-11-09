package plugins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"exodrive/internal/core"
	"exodrive/internal/fsutil"
)

type OrphanProvider struct{}

func NewOrphanProvider() *OrphanProvider {
	return &OrphanProvider{}
}

func (p *OrphanProvider) Name() string {
	return "orphan"
}

func (p *OrphanProvider) Detect(ctx context.Context) ([]core.CacheItem, error) {
	var items []core.CacheItem
	home, _ := os.UserHomeDir()

	// Check for orphaned go-build cache on local SSD when GOCACHE points to external
	localGoBuild := filepath.Join(home, "Library", "Caches", "go-build")
	activeGocache := os.Getenv("GOCACHE")
	if activeGocache != localGoBuild && activeGocache != "" {
		if fi, err := os.Stat(localGoBuild); err == nil && fi.IsDir() {
			size, _ := fsutil.DirSize(localGoBuild)
			if size > 1024*1024 { // larger than 1MB
				items = append(items, core.CacheItem{
					ID:             "orphan.go-build",
					ToolName:       "orphan",
					Title:          "Abandoned Local Go Build Cache",
					Category:       core.CategoryOrphan,
					CurrentPath:    localGoBuild,
					ConfiguredPath: "Active Go cache is redirected to " + activeGocache,
					SizeBytes:      size,
					IsActive:       false,
					IsMigrated:     false,
					IsSymlink:      false,
					RedirectMethod: core.RedirectManual,
					TargetSubdir:   "orphans/go-build",
					Warning:        "This legacy build cache is no longer used by Go. Safe to clean or archive.",
				})
			}
		}
	}

	// Check for orphaned local pnpm caches
	localPnpmCache := filepath.Join(home, "Library", "Caches", "pnpm")
	if fi, err := os.Stat(localPnpmCache); err == nil && fi.IsDir() {
		size, _ := fsutil.DirSize(localPnpmCache)
		if size > 1024*1024 && !fsutil.IsSymlink(localPnpmCache) && !strings.HasPrefix(localPnpmCache, "/Volumes/") {
			items = append(items, core.CacheItem{
				ID:             "orphan.pnpm-cache",
				ToolName:       "orphan",
				Title:          "Legacy Local PNPM Metadata Cache",
				Category:       core.CategoryOrphan,
				CurrentPath:    localPnpmCache,
				SizeBytes:      size,
				IsActive:       false,
				IsMigrated:     false,
				IsSymlink:      false,
				RedirectMethod: core.RedirectManual,
				TargetSubdir:   "orphans/pnpm-cache",
				Warning:        "Legacy local PNPM cache folder. Safe to clean.",
			})
		}
	}

	return items, nil
}

func (p *OrphanProvider) PreflightCheck(ctx context.Context, item core.CacheItem, targetRoot string) error {
	return nil
}

func (p *OrphanProvider) Migrate(ctx context.Context, item core.CacheItem, targetRoot string, onProgress fsutil.CopyProgressCallback) error {
	dest := filepath.Join(targetRoot, item.TargetSubdir)
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}

	if err := fsutil.CopyDir(item.CurrentPath, dest, item.SizeBytes, onProgress); err != nil {
		return fmt.Errorf("archive orphan: %w", err)
	}

	_ = os.RemoveAll(item.CurrentPath)

	return core.RecordMigration(core.MigrationRecord{
		ID:             item.ID,
		ToolName:       p.Name(),
		OriginalPath:   item.CurrentPath,
		DestinationDir: dest,
		RedirectMethod: core.RedirectManual,
		MigratedAt:     time.Now(),
		SizeBytes:      item.SizeBytes,
	})
}

func (p *OrphanProvider) Rollback(ctx context.Context, item core.CacheItem) error {
	m, err := core.LoadManifest()
	if err != nil {
		return err
	}
	rec, ok := m.Records[item.ID]
	if !ok {
		return fmt.Errorf("no migration record for %s", item.ID)
	}

	if err := fsutil.CopyDir(rec.DestinationDir, rec.OriginalPath, rec.SizeBytes, nil); err != nil {
		return err
	}

	return core.RemoveRecord(item.ID)
}
