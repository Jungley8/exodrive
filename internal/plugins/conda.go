package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"exodrive/internal/core"
	"exodrive/internal/fsutil"
)

// condaWarning says what a move costs. Conda hardlinks packages from the cache
// into an environment on the same volume. Across volumes it copies them.
const condaWarning = "Stop every conda command before the move. Conda copies packages across volumes, so environments use more space."

// condaProvider moves the conda package caches. A conda install keeps the cache
// in a prefix that only the tool knows, so the provider asks the tool.
type condaProvider struct{}

var _ Provider = (*condaProvider)(nil)

// NewCondaProvider returns the provider for conda.
func NewCondaProvider() *condaProvider { return &condaProvider{} }

func (p *condaProvider) Name() string { return "conda" }

// condaInfo is the part of `conda info --json` that names the package caches.
type condaInfo struct {
	PkgsDirs []string `json:"pkgs_dirs"`
}

func (p *condaProvider) Detect(ctx context.Context) ([]core.CacheItem, error) {
	raw, ok := toolQuery(ctx, "conda", "info", "--json")
	if !ok {
		return nil, nil
	}
	var info condaInfo
	if err := json.Unmarshal([]byte(raw), &info); err != nil {
		return nil, fmt.Errorf("read the conda package folders: %w", err)
	}

	items := make([]core.CacheItem, 0, len(info.PkgsDirs))
	seen := make(map[string]bool, len(info.PkgsDirs))
	for _, dir := range info.PkgsDirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		dir = filepath.Clean(dir)
		if seen[dir] {
			continue
		}
		seen[dir] = true
		stat, err := os.Stat(dir)
		if err != nil || !stat.IsDir() {
			continue
		}
		size, _ := fsutil.DirSize(ctx, dir)
		item := condaItem(dir)
		item.SizeBytes = size
		items = append(items, item)
	}
	return items, nil
}

// condaItem names one package cache. The ID and the target derive from the path,
// so a cache keeps its identity when another cache appears or goes away.
func condaItem(dir string) core.CacheItem {
	suffix := shortDigest(dir)
	title := "Conda Package Cache"
	if prefix := filepath.Base(filepath.Dir(dir)); prefix != "" && prefix != "." && prefix != string(filepath.Separator) {
		title = fmt.Sprintf("Conda Package Cache (%s)", prefix)
	}
	return core.CacheItem{
		ID:             "conda.pkgs-" + suffix,
		ToolName:       "conda",
		Title:          title,
		Category:       core.CategoryPackageStore,
		CurrentPath:    dir,
		ConfiguredPath: filepath.Dir(dir),
		IsActive:       true,
		IsMigrated:     movedOffVolume(dir),
		IsSymlink:      fsutil.IsSymlink(dir),
		RedirectMethod: core.RedirectSymlink,
		TargetSubdir:   filepath.Join("conda", "pkgs-"+suffix),
		Warning:        condaWarning,
	}
}

// shortDigest is a short name for a path. It holds the same value for the same
// path on every scan.
func shortDigest(path string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(path)))
	return hex.EncodeToString(sum[:4])
}

func (p *condaProvider) PreflightCheck(ctx context.Context, item core.CacheItem, targetRoot string) error {
	running, err := fsutil.CheckRunningProcesses("conda", "mamba", "micromamba")
	if err != nil {
		return err
	}
	if len(running) > 0 {
		return fmt.Errorf("%s is running (PID: %s): stop it before migration", running[0].Name, running[0].PID)
	}
	return nil
}

func (p *condaProvider) Migrate(ctx context.Context, item core.CacheItem, targetRoot string, progress fsutil.MigrateProgress) error {
	return moveAndLink("conda", item, filepath.Join(targetRoot, item.TargetSubdir), progress)
}

func (p *condaProvider) Rollback(ctx context.Context, item core.CacheItem) error {
	return rollbackLinked(item.ID)
}
