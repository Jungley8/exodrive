//go:build windows

package core

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// Exodrive calls LockFileEx itself, so the Windows build needs no extra module.
// The call blocks until the lock is free, which is what flock(LOCK_EX) does on
// macOS and Linux.
var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = kernel32.NewProc("LockFileEx")
	procUnlockFileEx = kernel32.NewProc("UnlockFileEx")
)

const (
	lockfileExclusiveLock = 0x00000002
	// The whole 64-bit range, so two writers cannot lock different parts.
	lockWholeFile = 0xFFFFFFFF
)

// lockManifest takes an exclusive lock on the ledger lock file, so two
// Exodrive processes cannot rewrite the ledger at the same time.
func lockManifest() (func(), error) {
	return lockPath(GetManifestPath())
}

// lockPath takes an exclusive lock on target + ".lock". The failure log uses
// its own lock file, so a report never waits on a migration.
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

	var ov syscall.Overlapped
	ok, _, callErr := procLockFileEx.Call(
		uintptr(f.Fd()),
		uintptr(lockfileExclusiveLock),
		0,
		uintptr(lockWholeFile),
		uintptr(lockWholeFile),
		uintptr(unsafe.Pointer(&ov)),
	)
	if ok == 0 {
		_ = f.Close()
		return nil, fmt.Errorf("lock %s: %w", p, callErr)
	}

	return func() {
		var unlock syscall.Overlapped
		_, _, _ = procUnlockFileEx.Call(
			uintptr(f.Fd()),
			0,
			uintptr(lockWholeFile),
			uintptr(lockWholeFile),
			uintptr(unsafe.Pointer(&unlock)),
		)
		_ = f.Close()
	}, nil
}
