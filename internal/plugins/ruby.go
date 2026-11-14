package plugins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"exodrive/internal/core"
	"exodrive/internal/fsutil"
)

// rubyBundlerWarning says what a move costs: Bundler downloads a gem again when
// a project needs it.
const rubyBundlerWarning = "Stop every bundle command before the move. Bundler downloads the gems again when it needs them."

// rubyGemWarning covers the downloaded .gem files that RubyGems keeps.
const rubyGemWarning = "Stop every gem command before the move. RubyGems downloads the gems again when it needs them."

// rubyProvider moves the Bundler cache and the cache of every user gem folder.
// A gem folder carries the Ruby version in its name, so the provider discovers
// the folders on each scan.
type rubyProvider struct{}

var _ Provider = (*rubyProvider)(nil)

// NewRubyProvider returns the provider for Ruby.
func NewRubyProvider() *rubyProvider { return &rubyProvider{} }

func (p *rubyProvider) Name() string { return "ruby" }

// bundlerCache returns the folder that holds Bundler's downloads.
// BUNDLE_USER_CACHE names it, and ~/.bundle/cache is the default.
func bundlerCache() string {
	if dir := strings.TrimSpace(os.Getenv("BUNDLE_USER_CACHE")); dir != "" {
		return dir
	}
	return homePath(".bundle", "cache")
}

func (p *rubyProvider) Detect(ctx context.Context) ([]core.CacheItem, error) {
	var items []core.CacheItem
	if item, ok := rubyItem(ctx, bundlerCache(), "ruby.bundler-cache", "Bundler Cache",
		filepath.Join("ruby", "bundler-cache"), rubyBundlerWarning); ok {
		items = append(items, item)
	}

	pattern := homePath(".gem", "ruby", "*", "cache")
	if pattern == "" {
		return items, nil
	}
	dirs, err := filepath.Glob(pattern)
	if err != nil {
		return items, fmt.Errorf("read the RubyGems folders: %w", err)
	}
	for _, dir := range dirs {
		suffix := shortDigest(dir)
		version := filepath.Base(filepath.Dir(dir))
		title := fmt.Sprintf("RubyGems Cache (%s)", version)
		item, ok := rubyItem(ctx, dir, "ruby.gem-cache-"+suffix, title,
			filepath.Join("ruby", "gem-cache-"+suffix), rubyGemWarning)
		if ok {
			items = append(items, item)
		}
	}
	return items, nil
}

// rubyItem builds one item for a folder that exists. The ID and the target
// derive from the path, so a gem cache keeps its identity across scans.
func rubyItem(ctx context.Context, dir, id, title, subdir, warning string) (core.CacheItem, bool) {
	if dir == "" {
		return core.CacheItem{}, false
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return core.CacheItem{}, false
	}
	size, _ := fsutil.DirSize(ctx, dir)
	return core.CacheItem{
		ID:             id,
		ToolName:       "ruby",
		Title:          title,
		Category:       core.CategoryPackageStore,
		CurrentPath:    dir,
		ConfiguredPath: filepath.Dir(dir),
		SizeBytes:      size,
		IsActive:       true,
		IsMigrated:     movedOffVolume(dir),
		IsSymlink:      fsutil.IsSymlink(dir),
		RedirectMethod: core.RedirectSymlink,
		TargetSubdir:   subdir,
		Warning:        warning,
	}, true
}

// PreflightCheck refuses a move while a Ruby package command runs. The lock
// names bundle and gem, not ruby: a Rails server is not a package command.
func (p *rubyProvider) PreflightCheck(ctx context.Context, item core.CacheItem, targetRoot string) error {
	running, err := fsutil.CheckRunningProcesses("bundle", "gem")
	if err != nil {
		return err
	}
	if len(running) > 0 {
		return fmt.Errorf("%s is running (PID: %s): stop it before migration", running[0].Name, running[0].PID)
	}
	return nil
}

func (p *rubyProvider) Migrate(ctx context.Context, item core.CacheItem, targetRoot string, progress fsutil.MigrateProgress) error {
	return moveAndLink("ruby", item, filepath.Join(targetRoot, item.TargetSubdir), progress)
}

func (p *rubyProvider) Rollback(ctx context.Context, item core.CacheItem) error {
	return rollbackLinked(item.ID)
}
