package plugins

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"exodrive/internal/core"
	"exodrive/internal/fsutil"
)

// moveAndLink copies item to dest, verifies the copy, records the move with its
// digest, then replaces the original path with a symbolic link. A single file
// lands inside dest under its own name.
func moveAndLink(tool string, item core.CacheItem, dest string, progress fsutil.MigrateProgress) error {
	if item.IsSymlink {
		return fmt.Errorf("%s is already a symlink: nothing to migrate", item.CurrentPath)
	}

	info, err := os.Stat(item.CurrentPath)
	if err != nil {
		return err
	}

	destination := dest
	var summary fsutil.CopySummary
	if info.IsDir() {
		if err := fsutil.CopyDir(item.CurrentPath, dest, item.SizeBytes, progress.Copy); err != nil {
			return fmt.Errorf("copy %s: %w", item.Title, err)
		}
		summary, err = fsutil.VerifyTreeSummary(item.CurrentPath, dest, progress.Verify)
		if err != nil {
			return fmt.Errorf("%s: %w", item.Title, err)
		}
	} else {
		destination = filepath.Join(dest, filepath.Base(item.CurrentPath))
		if err := fsutil.CopyFile(item.CurrentPath, destination, item.SizeBytes, progress.Copy); err != nil {
			return fmt.Errorf("copy %s: %w", item.Title, err)
		}
		summary, err = fsutil.VerifyFileSummary(item.CurrentPath, destination)
		if err != nil {
			return fmt.Errorf("%s: %w", item.Title, err)
		}
	}

	if err := core.RecordMigration(core.MigrationRecord{
		ID:             item.ID,
		ToolName:       tool,
		OriginalPath:   item.CurrentPath,
		DestinationDir: destination,
		RedirectMethod: core.RedirectSymlink,
		OriginalConfig: item.ConfiguredPath,
		MigratedAt:     time.Now(),
		SizeBytes:      item.SizeBytes,
		TreeDigest:     string(summary.Digest),
		FileCount:      summary.Files,
		TotalBytes:     summary.Bytes,
	}); err != nil {
		return err
	}

	if err := fsutil.ReplaceDirWithSymlink(item.CurrentPath, destination, item.CurrentPath+".bak"); err != nil {
		return switchFailed(item.ID, err)
	}

	return discardLocalBackup(item.CurrentPath)
}

// rollbackLinked copies the recorded copy back, then removes the ledger record.
func rollbackLinked(itemID string) error {
	rec, err := loadRecord(itemID)
	if err != nil {
		return err
	}

	if err := restoreSymlink(rec.OriginalPath, rec.DestinationDir); err != nil {
		return err
	}

	return core.RemoveRecord(itemID)
}
