package plugins

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"exodrive/internal/core"
	"exodrive/internal/fsutil"
)

// loadRecord returns the ledger record for id, or an error when none exists.
// A rollback must never run without its record.
func loadRecord(id string) (core.MigrationRecord, error) {
	m, err := core.LoadManifest()
	if err != nil {
		return core.MigrationRecord{}, fmt.Errorf("read migration ledger: %w", err)
	}
	rec, ok := m.Records[id]
	if !ok {
		return core.MigrationRecord{}, fmt.Errorf("no migration record for %s", id)
	}
	if err := core.ValidateRecord(rec); err != nil {
		return core.MigrationRecord{}, err
	}
	return rec, nil
}

// runCommand runs a tool's native configuration command and returns its output
// when the command fails. Callers must check the result: a rollback that cannot
// restore the tool's configuration must not report success.
func runCommand(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
		}
		return fmt.Errorf("%s %s: %w (%s)", name, strings.Join(args, " "), err, msg)
	}
	return nil
}

// envOverride refuses a migration whose native configuration cannot take
// effect, because the named environment variable pins the tool to a path. The
// tool would keep using the local cache after Exodrive deleted it.
func envOverride(names ...string) error {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return fmt.Errorf("%s is set to %q: unset it before migration, or %s keeps that path", name, value, name)
		}
	}
	return nil
}

// discardLocalBackup removes the local copy that a swap set aside at
// path+".bak". A migration calls it after it records the ledger entry, so a
// completed migration frees the local space it moved.
func discardLocalBackup(original string) error {
	backup := original + ".bak"
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("remove local copy %s: %w", backup, err)
	}
	return nil
}

// removeLocalSource deletes the local cache that a native-config migration
// moved to destination. It refuses any path that holds the destination.
func removeLocalSource(original, destination string) error {
	if original == destination || strings.HasPrefix(original, destination+string(filepath.Separator)) {
		return fmt.Errorf("refusing to delete %s: it holds the destination", original)
	}
	if err := os.RemoveAll(original); err != nil {
		return fmt.Errorf("remove local copy %s: %w", original, err)
	}
	return nil
}

// switchFailed clears the ledger record after a failed switch, so the user can
// retry the migration. The copied data stays at the destination.
func switchFailed(id string, cause error) error {
	if rbErr := core.RemoveRecord(id); rbErr != nil {
		return fmt.Errorf("%w; also clear the ledger record for %s: %v", cause, id, rbErr)
	}
	return cause
}

// stageRestore copies destination to a sibling of original and checks the copy.
// It returns the staged path, which the caller renames into place.
func stageRestore(destination, original string) (staging string, err error) {
	staging = original + ".restore"
	if err := os.RemoveAll(staging); err != nil {
		return "", fmt.Errorf("rollback: clear %s: %w", staging, err)
	}

	info, err := os.Stat(destination)
	if err != nil {
		return "", fmt.Errorf("rollback: destination %s is unavailable: %w", destination, err)
	}

	if info.IsDir() {
		if err := fsutil.CopyDir(destination, staging, 0, nil); err != nil {
			_ = os.RemoveAll(staging)
			return "", fmt.Errorf("rollback: copy %s back: %w", destination, err)
		}
		if err := fsutil.VerifyTree(destination, staging); err != nil {
			_ = os.RemoveAll(staging)
			return "", fmt.Errorf("rollback: %w", err)
		}
	} else {
		if err := fsutil.CopyFile(destination, staging, 0, nil); err != nil {
			_ = os.RemoveAll(staging)
			return "", fmt.Errorf("rollback: copy %s back: %w", destination, err)
		}
		if err := fsutil.VerifyFile(destination, staging); err != nil {
			_ = os.RemoveAll(staging)
			return "", fmt.Errorf("rollback: %w", err)
		}
	}
	return staging, nil
}

// restoreInto copies the migrated data from destination back to original and
// removes destination. original must not exist yet.
func restoreInto(original, destination string) error {
	staging, err := stageRestore(destination, original)
	if err != nil {
		return err
	}
	if err := os.Rename(staging, original); err != nil {
		_ = os.RemoveAll(staging)
		return fmt.Errorf("rollback: restore %s: %w", original, err)
	}
	if err := os.RemoveAll(destination); err != nil {
		return fmt.Errorf("rollback: remove %s: %w", destination, err)
	}
	return nil
}

// restoreSymlink replaces the symlink at original with the data held at
// destination. The copy is staged first, so a failure leaves the link working.
func restoreSymlink(original, destination string) error {
	if !fsutil.IsSymlink(original) {
		return reconcileOriginal(original, destination)
	}

	staging, err := stageRestore(destination, original)
	if err != nil {
		return err
	}

	if err := os.Remove(original); err != nil {
		_ = os.RemoveAll(staging)
		return fmt.Errorf("rollback: remove symlink %s: %w", original, err)
	}
	if err := os.Rename(staging, original); err != nil {
		// Put the link back so the tool keeps working.
		_ = fsutil.AtomicSymlink(destination, original)
		_ = os.RemoveAll(staging)
		return fmt.Errorf("rollback: restore %s: %w", original, err)
	}
	if err := os.RemoveAll(destination); err != nil {
		return fmt.Errorf("rollback: remove %s: %w", destination, err)
	}
	return nil
}

// reconcileOriginal finishes a rollback whose local path is no longer a
// symlink. It happens when the switch never ran, or when an earlier rollback
// already copied the data back. The two trees must match before Exodrive
// deletes the destination.
func reconcileOriginal(original, destination string) error {
	if _, err := os.Stat(original); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("rollback: %s is gone and is not a symlink: nothing to restore", original)
		}
		return fmt.Errorf("rollback: inspect %s: %w", original, err)
	}
	if err := verifySameData(destination, original); err != nil {
		return fmt.Errorf("rollback: %s exists and differs from %s: %w", original, destination, err)
	}
	if err := os.RemoveAll(destination); err != nil {
		return fmt.Errorf("rollback: remove %s: %w", destination, err)
	}
	return nil
}

// verifySameData compares the copy on the drive with the path on this machine.
// A tree check on a single file compares nothing, so a file row needs the file
// check: the digest would pass on two empty walks.
func verifySameData(destination, original string) error {
	info, err := os.Stat(destination)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fsutil.VerifyFile(destination, original)
	}
	return fsutil.VerifyTree(destination, original)
}

// restoreNativeData moves the migrated data back after the tool configuration
// points at the original path again. A local tree that matches the destination
// comes from an earlier, interrupted rollback, so Exodrive only removes the
// destination. A different tree keeps both and reports the conflict.
func restoreNativeData(rec core.MigrationRecord) error {
	if _, err := os.Stat(rec.DestinationDir); err != nil {
		return fmt.Errorf("rollback: destination %s is unavailable: %w", rec.DestinationDir, err)
	}

	switch _, err := os.Stat(rec.OriginalPath); {
	case err == nil:
		return reconcileOriginal(rec.OriginalPath, rec.DestinationDir)
	case os.IsNotExist(err):
		return restoreInto(rec.OriginalPath, rec.DestinationDir)
	default:
		return fmt.Errorf("rollback: inspect %s: %w", rec.OriginalPath, err)
	}
}
