//go:build unix

package core

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// lockManifest takes an exclusive advisory lock on the ledger lock file, so
// two Exodrive processes cannot rewrite the ledger at the same time.
func lockManifest() (func(), error) {
	return lockPath(GetManifestPath())
}

// lockPath takes an exclusive advisory lock on target + ".lock". The failure
// log uses its own lock file, so a report never waits on a migration.
func lockPath(target string) (func(), error) {
	p := target + ".lock"
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open lock %s: %w", p, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("lock %s: %w", p, err)
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
