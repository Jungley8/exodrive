package core

import (
	"errors"
	"testing"
)

func TestFailureKindNamesWhatStopped(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"target", &TargetError{Volume: "SSD", Message: "the volume is gone"}, KindTarget},
		{"unsupported", errors.New("fsutil: exFAT is not supported; use APFS"), KindUnsupported},
		{"incompatible", errors.New("incompatible filesystem: ntfs"), KindUnsupported},
		{"space", errors.New("insufficient space on the target"), KindSpace},
		{"running", errors.New("npm is running; stop it first"), KindRunning},
		{"symlink", errors.New("~/.npm is already a symlink"), KindSymlink},
		{"verification", errors.New("verification failed: a file differs"), KindVerification},
		{"env", errors.New("GOCACHE is set to /tmp/cache"), KindEnv},
		{"no record", errors.New("no record for npm.cache"), KindNoRecord},
		{"no migration record", errors.New("no migration record"), KindNoRecord},
		{"ledger", errors.New("read migration ledger: bad json"), KindLedger},
		{"manifest", errors.New("write manifest: disk full"), KindLedger},
		{"differs", errors.New("both copies exists and differs"), KindDiffers},
		{"missing folder", errors.New("the folder does not exist"), KindMissing},
		{"missing tool", errors.New("npm is not available"), KindMissing},
		{"other", errors.New("something else"), KindOther},
	}
	for _, c := range cases {
		if got := FailureKind(c.err); got != c.want {
			t.Errorf("%s: FailureKind = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestFailureKindReadsAWrappedTargetError(t *testing.T) {
	err := errors.Join(errors.New("move npm.cache"), &TargetError{Path: "/Volumes/SSD", Message: "gone"})
	if got := FailureKind(err); got != KindTarget {
		t.Fatalf("FailureKind = %q, want %q", got, KindTarget)
	}
}
