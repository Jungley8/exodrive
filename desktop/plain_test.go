package desktop

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	exoCore "exodrive/internal/core"
)

// TestExplainNamesTheCauseAndTheNextStep pins every reason the window can show
// to the words a person reads. The errors are the ones the engine and the
// providers write, so a change to their wording fails here instead of in the
// window.
func TestExplainNamesTheCauseAndTheNextStep(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		action string
		title  string
		body   string
		hint   string
	}{
		{
			name: "unsupported drive", action: actionMove,
			err:   errors.New("filesystem 'exfat' is not supported: choose APFS, HFS+, ext2/3/4, btrfs, zfs, xfs, f2fs, or bcachefs"),
			title: "Exodrive cannot use this drive",
			body:  "The drive uses a format that Exodrive cannot use for caches.",
			hint:  "Choose a folder on another drive. Exodrive supports APFS, HFS+, ext2, ext3, ext4, btrfs, zfs, xfs, f2fs, and bcachefs.",
		},
		{
			name: "full drive", action: actionMove,
			err:   errors.New("insufficient space on destination: need 4096 MB, only 12 MB available"),
			title: "The drive is full",
			body:  "The drive does not have enough free space for this cache.",
			hint:  "Free space on the drive, or move fewer caches at a time.",
		},
		{
			name: "running program", action: actionMove,
			err:   errors.New("npm is running (PID: 4711): stop active npm processes before migration"),
			title: "A program still uses this cache",
			body:  "A program that writes to this cache is open.",
			hint:  "Quit the program, then try again.",
		},
		{
			name: "links elsewhere", action: actionMove,
			err:   errors.New("/Users/me/.npm is already a symlink: nothing to migrate"),
			title: "This folder already links elsewhere",
			body:  "The folder points to a copy that Exodrive did not record.",
			hint:  "Run a scan. If the row keeps this message, the cache needs no move.",
		},
		{
			name: "copy differs", action: actionMove,
			err:   errors.New("verification failed: /Drive/npm has 12 files, /Users/me/.npm has 13"),
			title: "The copy did not match",
			body:  "The copy on the drive differs from the original.",
			hint:  "Exodrive deleted nothing. Try again. If it fails again, check the drive.",
		},
		{
			name: "shell variable", action: actionMove,
			err:   errors.New(`NPM_CONFIG_CACHE is set to "/tmp/cache": unset it before migration, or npm keeps that path`),
			title: "Your shell sets the cache path",
			body:  "A variable in your shell overrides the setting that Exodrive changes.",
			hint:  "Remove NPM_CONFIG_CACHE from your shell, then try again.",
		},
		{
			name: "record file", action: actionMove,
			err:   errors.New("open /Users/me/.exodrive/manifest.json: permission denied"),
			title: "Exodrive cannot read its move records",
			body:  "The record file is missing or unreadable.",
			hint:  "Check that the record file is readable, then scan again.",
		},
		{
			// The engine writes "no record found for x in manifest". The
			// missing record must win over the record file, so the person
			// does not look for a file that is there.
			name: "missing record", action: actionMoveBack,
			err:   errors.New("no record found for npm.cache in manifest"),
			title: "No record of this move",
			body:  "Exodrive cannot find the move that put this cache on the drive.",
			hint:  "Run a scan, then choose the cache again.",
		},
		{
			name: "two copies", action: actionMoveBack,
			err:   errors.New("rollback: /Users/me/.npm exists and differs from /Drive/npm: not empty"),
			title: "Two copies hold different data",
			body:  "The folder on this Mac and the copy on the drive are not the same.",
			hint:  "Both copies are safe. Exodrive stops so it cannot pick one for you.",
		},
		{
			name: "folder gone", action: actionDelete,
			err:   errors.New("open /tmp/cache: no such file or directory: folder does not exist"),
			title: "The folder is gone",
			body:  "The folder that Exodrive needs is not there.",
			hint:  "Choose the folder again, then try again.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := explain(tc.err, tc.action)
			if got.Title != tc.title {
				t.Fatalf("title is %q, want %q", got.Title, tc.title)
			}
			if got.Body != tc.body {
				t.Fatalf("body is %q, want %q", got.Body, tc.body)
			}
			if got.Hint != tc.hint {
				t.Fatalf("hint is %q, want %q", got.Hint, tc.hint)
			}
		})
	}
}

// TestExplainKeepsTheTitleByAction checks that one error reads as the action
// that stopped, and that an error with no match keeps its own words.
func TestExplainKeepsTheTitleByAction(t *testing.T) {
	unknown := errors.New("cp: read /tmp/cache: input/output error")

	for action, want := range map[string]string{
		actionMove:     "The move stopped",
		actionMoveBack: "The move back stopped",
		actionDelete:   "The delete stopped",
	} {
		got := explain(unknown, action)
		if got.Title != want {
			t.Fatalf("%s: title is %q, want %q", action, got.Title, want)
		}
		if got.Body != unknown.Error() {
			t.Fatalf("%s: body is %q, want the original text", action, got.Body)
		}
	}

	if got := explain(nil, actionMove); got.Title != "" || got.Body != "" || got.Hint != "" {
		t.Fatalf("a nil error made a message: %+v", got)
	}
}

// TestExplainSplitsATargetError keeps the statement and the next step on their
// own lines.
func TestExplainSplitsATargetError(t *testing.T) {
	err := &exoCore.TargetError{Volume: "SSD", Message: "The external volume SSD is not connected. Connect it, then check again."}
	got := explain(err, actionMove)
	if got.Body != "The external volume SSD is not connected." {
		t.Fatalf("body is %q", got.Body)
	}
	if got.Hint != "Connect it, then check again." {
		t.Fatalf("hint is %q", got.Hint)
	}

	one := explain(errors.New("one sentence only"), actionMove)
	if one.Hint != "Exodrive deleted nothing. Try again." {
		t.Fatalf("an error without a second sentence lost its next step: %q", one.Hint)
	}
}

func TestSplitSentences(t *testing.T) {
	body, hint := splitSentences("The first part. The second part.")
	if body != "The first part." || hint != "The second part." {
		t.Fatalf("split gave (%q, %q)", body, hint)
	}
	body, hint = splitSentences("Only one part")
	if body != "Only one part" || hint != "" {
		t.Fatalf("split gave (%q, %q)", body, hint)
	}
}

// TestTargetProblemSpeaksToTheChooser keeps the folder check in the window's
// words and keeps the path that the person typed.
func TestTargetProblemSpeaksToTheChooser(t *testing.T) {
	if got := targetProblem(nil); got != "" {
		t.Fatalf("a folder that passed the check got a problem: %q", got)
	}
	got := targetProblem(&exoCore.TargetError{Message: "The folder /Volumes/Old does not exist. Choose a folder that exists."})
	if !strings.HasPrefix(got, "The folder /Volumes/Old does not exist.") {
		t.Fatalf("a target error lost its detail: %q", got)
	}
	unsupported := targetProblem(errors.New("filesystem 'msdos' is not supported: choose APFS"))
	if !strings.HasPrefix(unsupported, "This drive cannot hold caches.") {
		t.Fatalf("an unsupported drive reads as %q", unsupported)
	}
	missing := targetProblem(errors.New("open /Volumes/Old: does not exist"))
	if !strings.HasPrefix(missing, "This folder does not exist.") {
		t.Fatalf("a missing folder reads as %q", missing)
	}
	raw := targetProblem(errors.New("stat /Volumes/X: input/output error"))
	if raw != "stat /Volumes/X: input/output error" {
		t.Fatalf("an unknown problem hid its reason: %q", raw)
	}
}

// TestShortPathHidesOnlyTheHomeFolder keeps a path readable and never shortens
// a folder that only shares the first letters of the home folder.
func TestShortPathHidesOnlyTheHomeFolder(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home folder")
	}
	if got := shortPath(home); got != "~" {
		t.Fatalf("home is %q, want ~", got)
	}
	if got := shortPath(filepath.Join(home, "Library", "Caches")); got != "~/Library/Caches" {
		t.Fatalf("home subfolder is %q", got)
	}
	sibling := home + "-backup"
	if got := shortPath(sibling); got != sibling {
		t.Fatalf("%q became %q; a folder outside home lost its path", sibling, got)
	}
	outside := "/Volumes/PHD/caches"
	if got := shortPath(outside); got != outside {
		t.Fatalf("%q became %q", outside, got)
	}
}

// TestStateOfReadsTheMoveFirst keeps one row to one state: a moved cache shows
// where it lives, not why it would not move.
func TestStateOfReadsTheMoveFirst(t *testing.T) {
	moved := exoCore.CacheItem{ID: "x", IsMigrated: true, IsActive: true, BlockedReason: "in use"}
	if got := stateOf(moved); got != stateMoved || got.label() != "On the drive" {
		t.Fatalf("a moved cache reads as %v", got)
	}
	unused := exoCore.CacheItem{ID: "x", IsActive: false, BlockedReason: "in use"}
	if got := stateOf(unused); got != stateUnused || got.label() != "Unused" {
		t.Fatalf("an unused cache reads as %v", got)
	}
	blocked := exoCore.CacheItem{ID: "x", IsActive: true, BlockedReason: "in use"}
	if got := stateOf(blocked); got != stateBlocked || got.label() != "Cannot move" {
		t.Fatalf("a blocked cache reads as %v", got)
	}
	local := exoCore.CacheItem{ID: "x", IsActive: true}
	if got := stateOf(local); got != stateLocal || got.label() != "On this Mac" {
		t.Fatalf("a local cache reads as %v", got)
	}
}

// TestMovableRefusesAUsedOrBlockedCache guards the gate that every move, select
// all, and count of "can move" reads.
func TestMovableRefusesAUsedOrBlockedCache(t *testing.T) {
	cases := []struct {
		name string
		item exoCore.CacheItem
		want bool
	}{
		{"local", exoCore.CacheItem{ID: "x", IsActive: true}, true},
		{"moved", exoCore.CacheItem{ID: "x", IsMigrated: true, IsActive: true}, false},
		{"unused", exoCore.CacheItem{ID: "x", IsActive: false}, false},
		{"blocked", exoCore.CacheItem{ID: "x", IsActive: true, BlockedReason: "in use"}, false},
	}
	for _, tc := range cases {
		if got := movable(tc.item); got != tc.want {
			t.Fatalf("%s: movable is %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestTallyCountsEachPlaceOnce(t *testing.T) {
	items := []exoCore.CacheItem{
		{ID: "a", IsActive: true, SizeBytes: 100},
		{ID: "b", IsActive: true, BlockedReason: "in use", SizeBytes: 200},
		{ID: "c", IsMigrated: true, IsActive: true, SizeBytes: 400},
		{ID: "d", IsActive: false, SizeBytes: 800},
	}
	local, moved, unused, canMove := tally(items)
	if local != 300 {
		t.Fatalf("local is %d, want 300 (a blocked cache still sits on this Mac)", local)
	}
	if moved != 400 || unused != 800 {
		t.Fatalf("moved is %d, unused is %d", moved, unused)
	}
	if canMove != 1 {
		t.Fatalf("canMove is %d, want 1", canMove)
	}

	local, moved, unused, canMove = tally(nil)
	if local != 0 || moved != 0 || unused != 0 || canMove != 0 {
		t.Fatalf("an empty list tallies to %d, %d, %d, %d", local, moved, unused, canMove)
	}
}

func TestCountKeepsThePlural(t *testing.T) {
	if got := count(1, "cache", "caches"); got != "1 cache" {
		t.Fatalf("count(1) is %q", got)
	}
	if got := count(0, "cache", "caches"); got != "0 caches" {
		t.Fatalf("count(0) is %q", got)
	}
	if got := count(2, "cache", "caches"); got != "2 caches" {
		t.Fatalf("count(2) is %q", got)
	}
}
