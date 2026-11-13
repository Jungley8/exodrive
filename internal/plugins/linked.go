package plugins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"exodrive/internal/core"
	"exodrive/internal/fsutil"
)

// linkedRow names one folder that a tool reads through a fixed path. The tool
// keeps that path after the move, because a symbolic link takes the place of
// the folder.
type linkedRow struct {
	id       string
	title    string
	path     func() string
	category core.CacheCategory
	subdir   string
	warning  string
	// blocked says why Exodrive never moves this folder. An empty value means
	// the folder can move.
	blocked string
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
		if err != nil || !info.IsDir() {
			continue
		}

		size, _ := fsutil.DirSize(ctx, path)
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
	running, err := fsutil.CheckRunningProcesses(p.lock...)
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
