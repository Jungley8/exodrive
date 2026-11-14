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
		rec, ok := manifest.Records[allItems[i].ID]
		if !ok {
			continue
		}
		// A record that names another folder does not describe this item: the
		// tool reads a path that no record covers, and the copy belongs to a
		// folder that the tool no longer reads. A move claim here would hide
		// both, so the scan stops the row and names the way out.
		if filepath.Clean(rec.OriginalPath) != filepath.Clean(allItems[i].CurrentPath) {
			if allItems[i].BlockedReason == "" {
				allItems[i].BlockedReason = driftReason(rec, allItems[i].CurrentPath)
			}
			continue
		}
		allItems[i].IsMigrated = true
		allItems[i].DestinationPath = rec.DestinationDir
		allItems[i].LinkState = linkStateOf(rec)
	}
	return allItems, errors.Join(detectErrs...)
}

// driftReason tells the person that a record describes another folder. The
// record still holds the copy, so a rollback returns it.
func driftReason(rec MigrationRecord, current string) string {
	return fmt.Sprintf(
		"Exodrive recorded a move of %s. This tool reads %s. Run 'exodrive rollback %s' to return the copy.",
		rec.OriginalPath, current, rec.ID)
}

// linkStateOf reads the original path of a record. It inspects only a record
// that moved the data by link, because a tool setting leaves no link to read.
// An original path that is gone is not a problem: the copy holds the data, and
// the tool writes a new folder when it needs one. A path that Exodrive cannot
// read counts as written again, so the card asks the person to look.
func linkStateOf(rec MigrationRecord) LinkState {
	if rec.RedirectMethod != RedirectSymlink {
		return LinkUnknown
	}
	info, err := os.Lstat(rec.OriginalPath)
	if err != nil {
		if os.IsNotExist(err) {
			return LinkUnknown
		}
		return LinkRewritten
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return LinkRewritten
	}
	target, err := os.Readlink(rec.OriginalPath)
	if err != nil {
		return LinkRewritten
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(rec.OriginalPath), target)
	}
	if filepath.Clean(target) == filepath.Clean(rec.DestinationDir) {
		return LinkMoved
	}
	return LinkRewritten
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
	// SameVolume is true when the folder sits on the volume that holds this
	// computer's home folder. A move into that volume frees no space, so the
	// window and the command line warn. Neither one stops the move.
	SameVolume bool `json:"same_volume"`
}

// TargetOnSystemVolume reports whether target sits on the volume that holds the
// home folder. The caches live there, so a move into that volume frees no
// space. An unreadable path reports false: the window only warns on a known
// answer.
func TargetOnSystemVolume(target string) bool {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return false
	}
	same, err := fsutil.IsSameDevice(target, home)
	return err == nil && same
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
		return TargetInfo{}, fmt.Errorf("filesystem '%s' is not supported: choose %s", fstype, fsutil.FilesystemHint())
	}
	return TargetInfo{Path: targetRoot, Filesystem: fstype, FreeBytes: freeBytes, TotalBytes: totalBytes,
		SameVolume: TargetOnSystemVolume(targetRoot)}, nil
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
		return fmt.Errorf("incompatible filesystem '%s': target storage requires %s", fstype, fsutil.FilesystemHint())
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

// KeepLocal finishes a move whose tool replaced the link with a real folder.
// The folder on this Mac stays, the copy on the drive goes, and the record goes
// with it. A tool that writes its cache again after a move causes this case.
// Every check runs before the first delete.
func (e *Engine) KeepLocal(id string) error {
	m, err := LoadManifest()
	if err != nil {
		return err
	}
	rec, ok := m.Records[id]
	if !ok {
		return fmt.Errorf("no record found for %s in manifest", id)
	}
	if err := ValidateRecord(rec); err != nil {
		return err
	}
	original := filepath.Clean(rec.OriginalPath)
	destination := filepath.Clean(rec.DestinationDir)

	info, err := os.Lstat(original)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("keep local: %s is gone; run the rollback to restore it from the drive", original)
		}
		return fmt.Errorf("keep local: inspect %s: %w", original, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("keep local: %s still links to the copy; run the rollback instead", original)
	}

	// The parent tells a detached drive from a copy that someone deleted. A
	// record must never go while the copy is out of reach.
	if _, err := os.Stat(filepath.Dir(destination)); err != nil {
		return fmt.Errorf("keep local: the drive that holds %s is not reachable: %w", destination, err)
	}
	if err := os.RemoveAll(destination); err != nil {
		return fmt.Errorf("keep local: remove %s: %w", destination, err)
	}
	return RemoveRecord(id)
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
	// The depth counts from the volume root, so it holds on Windows too.
	root := filepath.VolumeName(cleanPath) + string(filepath.Separator)
	rel, err := filepath.Rel(root, cleanPath)
	if err != nil || rel == "." {
		return fmt.Errorf("clean orphan: invalid path hierarchy: %q", path)
	}
	segments := strings.Split(rel, string(filepath.Separator))
	if len(segments) < 3 {
		return fmt.Errorf("clean orphan: refusing to delete shallow root directory: %q", path)
	}

	return os.RemoveAll(cleanPath)
}
