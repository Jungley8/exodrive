package core

import (
	"errors"
	"strings"
)

// Failure kinds are stable tokens. The window and the failure log both read
// them, so a person can report one word that names what stopped.
const (
	KindTarget       = "target"       // the chosen folder, through TargetError
	KindUnsupported  = "unsupported"  // the filesystem
	KindSpace        = "space"        // free space on the drive
	KindRunning      = "running"      // a program still uses the cache
	KindSymlink      = "symlink"      // the cache is already a link
	KindVerification = "verification" // the copy does not match
	KindEnv          = "env"          // a shell variable points at the cache
	KindNoRecord     = "no record"    // the ledger holds no such move
	KindLedger       = "ledger"       // the ledger cannot be read or written
	KindDiffers      = "differs"      // two copies both exist and differ
	KindMissing      = "missing"      // the folder or the tool is gone
	KindOther        = "other"        // an error that matches no rule
)

// FailureKind names what stopped an error. The rules read the text that the
// engine and the providers write, so this table must follow their words. The
// order matters: a later rule never hides an earlier one.
func FailureKind(err error) string {
	if err == nil {
		return ""
	}
	var target *TargetError
	if errors.As(err, &target) {
		return KindTarget
	}

	raw := err.Error()
	switch {
	case strings.Contains(raw, "is not supported"), strings.Contains(raw, "incompatible filesystem"):
		return KindUnsupported
	case strings.Contains(raw, "insufficient space"):
		return KindSpace
	case strings.Contains(raw, "is running"):
		return KindRunning
	case strings.Contains(raw, "is already a symlink"):
		return KindSymlink
	case strings.Contains(raw, "verification failed"):
		return KindVerification
	case strings.Contains(raw, " is set to "):
		return KindEnv
	case strings.Contains(raw, "no record"), strings.Contains(raw, "no migration record"):
		return KindNoRecord
	case strings.Contains(raw, "ledger"), strings.Contains(raw, "manifest"):
		return KindLedger
	case strings.Contains(raw, "exists and differs"):
		return KindDiffers
	case strings.Contains(raw, "does not exist"), strings.Contains(raw, "not available"):
		return KindMissing
	}
	return KindOther
}
