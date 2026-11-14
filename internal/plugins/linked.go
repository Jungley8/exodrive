package plugins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"exodrive/internal/core"
	"exodrive/internal/fsutil"
)

// linkedRow names one path that a tool reads through a fixed address. The tool
// keeps that path after the move, because a symbolic link takes the place of
// the folder or the file.
type linkedRow struct {
	id       string
	title    string
	path     func() string
	category core.CacheCategory
	subdir   string
	warning  string
	// file marks a row that moves one file instead of a folder.
	file bool
	// check refuses a move that would strand data outside the path, such as a
	// database journal. An empty value means the path needs no extra check.
	check func(path string) error
	// blocked says why Exodrive never moves this folder. An empty value means
	// the folder can move.
	blocked string
	// lock names the processes that write this folder, when they differ from
	// the processes of the other rows of the provider. An empty value means the
	// provider list applies.
	lock []string
}

// linkedProvider moves a fixed set of folders and replaces each with a
// symbolic link. One provider serves every row with the same tool name.
type linkedProvider struct {
	tool string
	lock []string
	rows []linkedRow
}

var _ Provider = (*linkedProvider)(nil)

func (p *linkedProvider) Name() string { return p.tool }

func (p *linkedProvider) Detect(ctx context.Context) ([]core.CacheItem, error) {
	items := make([]core.CacheItem, 0, len(p.rows))
	for _, row := range p.rows {
		path := row.path()
		if path == "" {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		// A row names one shape. A folder in the place of a file row is another
		// program's folder, and a file in the place of a folder row is not the
		// store this provider describes.
		if info.IsDir() == row.file {
			continue
		}

		var size int64
		if row.file {
			size = info.Size()
		} else {
			size, _ = fsutil.DirSize(ctx, path)
		}
		blocked := row.blocked != ""
		items = append(items, core.CacheItem{
			ID:             row.id,
			ToolName:       p.tool,
			Title:          row.title,
			Category:       row.category,
			CurrentPath:    path,
			ConfiguredPath: filepath.Dir(path),
			SizeBytes:      size,
			IsActive:       true,
			// A blocked folder never holds a record of a move.
			IsMigrated:     !blocked && movedOffVolume(path),
			IsSymlink:      fsutil.IsSymlink(path),
			RedirectMethod: core.RedirectSymlink,
			TargetSubdir:   row.subdir,
			Warning:        row.warning,
			BlockedReason:  row.blocked,
		})
	}
	return items, nil
}

func (p *linkedProvider) PreflightCheck(ctx context.Context, item core.CacheItem, targetRoot string) error {
	if item.BlockedReason != "" {
		return fmt.Errorf("%s: %s", item.Title, item.BlockedReason)
	}
	if row, ok := p.rowFor(item.ID); ok && row.check != nil {
		if err := row.check(item.CurrentPath); err != nil {
			return err
		}
	}
	running, err := fsutil.CheckRunningProcesses(p.locksFor(item.ID)...)
	if err != nil {
		return err
	}
	if len(running) > 0 {
		return fmt.Errorf("%s is running (PID: %s): stop it before migration", running[0].Name, running[0].PID)
	}
	return nil
}

func (p *linkedProvider) Migrate(ctx context.Context, item core.CacheItem, targetRoot string, progress fsutil.MigrateProgress) error {
	if item.BlockedReason != "" {
		return fmt.Errorf("%s: %s", item.Title, item.BlockedReason)
	}
	return moveAndLink(p.tool, item, filepath.Join(targetRoot, item.TargetSubdir), progress)
}

func (p *linkedProvider) Rollback(ctx context.Context, item core.CacheItem) error {
	return rollbackLinked(item.ID)
}

// homePath joins the folders under the home folder. It returns "" when the home
// folder is unknown, so a scan reports nothing instead of a guessed path.
func homePath(parts ...string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(append([]string{home}, parts...)...)
}

// under joins a relative folder to a root that may be empty.
func under(root, rel string) string {
	if root == "" {
		return ""
	}
	return filepath.Join(root, rel)
}

// rowFor returns the row that names one path.
func (p *linkedProvider) rowFor(id string) (linkedRow, bool) {
	for _, row := range p.rows {
		if row.id == id {
			return row, true
		}
	}
	return linkedRow{}, false
}

// locksFor names the processes that write one row. A row that names its own
// processes keeps a move of another row from waiting on them.
func (p *linkedProvider) locksFor(id string) []string {
	if row, ok := p.rowFor(id); ok && len(row.lock) > 0 {
		return row.lock
	}
	return p.lock
}
