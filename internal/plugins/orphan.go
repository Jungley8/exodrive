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

type OrphanProvider struct{}

func NewOrphanProvider() *OrphanProvider {
	return &OrphanProvider{}
}

func (p *OrphanProvider) Name() string {
	return "orphan"
}

// goEnvVar reads a Go setting from the tool itself. `go env -w` stores a value
// that the shell does not export, so the environment alone is not reliable.
func goEnvVar(ctx context.Context, name string) string {
	out, err := exec.CommandContext(ctx, "go", "env", name).Output()
	if err != nil {
		return strings.TrimSpace(os.Getenv(name))
	}
	return strings.TrimSpace(string(out))
}

func (p *OrphanProvider) Detect(ctx context.Context) ([]core.CacheItem, error) {
	var items []core.CacheItem
	home, _ := os.UserHomeDir()

	// Check for orphaned go-build cache on local SSD when GOCACHE points to external
	localGoBuild := filepath.Join(home, "Library", "Caches", "go-build")
	activeGocache := goEnvVar(ctx, "GOCACHE")
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
					Warning:        "Legacy Go build cache. Clean or archive this item.",
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
				Warning:        "Legacy PNPM cache folder. Clean this item.",
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
	if err := fsutil.VerifyTree(item.CurrentPath, dest); err != nil {
		return fmt.Errorf("archive orphan: %w", err)
	}

	if err := core.RecordMigration(core.MigrationRecord{
		ID:             item.ID,
		ToolName:       p.Name(),
		OriginalPath:   item.CurrentPath,
		DestinationDir: dest,
		RedirectMethod: core.RedirectManual,
		MigratedAt:     time.Now(),
		SizeBytes:      item.SizeBytes,
	}); err != nil {
		return err
	}

	return removeLocalSource(item.CurrentPath, dest)
}

func (p *OrphanProvider) Rollback(ctx context.Context, item core.CacheItem) error {
	rec, err := loadRecord(item.ID)
	if err != nil {
		return err
	}

	if err := restoreNativeData(rec); err != nil {
		return err
	}

	return core.RemoveRecord(item.ID)
}
