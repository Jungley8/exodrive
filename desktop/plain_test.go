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
			got := explain(tc.err, tc.action, englishText)
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
		got := explain(unknown, action, englishText)
		if got.Title != want {
			t.Fatalf("%s: title is %q, want %q", action, got.Title, want)
		}
		if got.Body != unknown.Error() {
			t.Fatalf("%s: body is %q, want the original text", action, got.Body)
		}
	}

	if got := explain(nil, actionMove, englishText); got.Title != "" || got.Body != "" || got.Hint != "" {
		t.Fatalf("a nil error made a message: %+v", got)
	}
}

// TestTargetErrorKeepsBothSentences keeps the statement and the next step on
// their own lines, and names the folder when no volume answers.
func TestTargetErrorKeepsBothSentences(t *testing.T) {
	err := &exoCore.TargetError{Volume: "SSD", Path: "/Volumes/SSD/caches",
		Message: "The external volume SSD is not connected. Connect it, then check again."}
	got := explain(err, actionMove, englishText)
	if got.Body != "The external volume SSD is not connected." {
		t.Fatalf("body is %q", got.Body)
	}
	if got.Hint != "Connect it, then check again." {
		t.Fatalf("hint is %q", got.Hint)
	}

	folder := explain(&exoCore.TargetError{Path: "/Volumes/Old/caches"}, actionChoose, englishText)
	if folder.Body != "The folder /Volumes/Old/caches does not exist." {
		t.Fatalf("a missing folder reads as %q", folder.Body)
	}
	if folder.Hint != "Choose a folder that exists." {
		t.Fatalf("a missing folder suggests %q", folder.Hint)
	}

	one := explain(errors.New("one sentence only"), actionMove, englishText)
	if one.Hint != "Exodrive deleted nothing. Try again." {
		t.Fatalf("an error without a second sentence lost its next step: %q", one.Hint)
	}
}

// TestTargetProblemSpeaksToTheChooser keeps the folder check in the window's
// words and keeps the path that the person typed.
func TestTargetProblemSpeaksToTheChooser(t *testing.T) {
	if got := targetProblem(nil, englishText); got != "" {
		t.Fatalf("a folder that passed the check got a problem: %q", got)
	}
	got := targetProblem(&exoCore.TargetError{Path: "/Volumes/Old"}, englishText)
	if !strings.HasPrefix(got, "The folder /Volumes/Old does not exist.") {
		t.Fatalf("a target error lost its detail: %q", got)
	}
	unsupported := targetProblem(errors.New("filesystem 'msdos' is not supported: choose APFS"), englishText)
	if !strings.HasPrefix(unsupported, "This drive cannot hold caches.") {
		t.Fatalf("an unsupported drive reads as %q", unsupported)
	}
	missing := targetProblem(errors.New("open /Volumes/Old: does not exist"), englishText)
	if !strings.HasPrefix(missing, "This folder does not exist.") {
		t.Fatalf("a missing folder reads as %q", missing)
	}
	raw := targetProblem(errors.New("stat /Volumes/X: input/output error"), englishText)
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
	if got := stateOf(moved); got != stateMoved || got.label(englishText) != "On the drive" {
		t.Fatalf("a moved cache reads as %v", got)
	}
	unused := exoCore.CacheItem{ID: "x", IsActive: false, BlockedReason: "in use"}
	if got := stateOf(unused); got != stateUnused || got.label(englishText) != "Unused" {
		t.Fatalf("an unused cache reads as %v", got)
	}
	blocked := exoCore.CacheItem{ID: "x", IsActive: true, BlockedReason: "in use"}
	if got := stateOf(blocked); got != stateBlocked || got.label(englishText) != "Cannot move" {
		t.Fatalf("a blocked cache reads as %v", got)
	}
	local := exoCore.CacheItem{ID: "x", IsActive: true}
	if got := stateOf(local); got != stateLocal || got.label(englishText) != "On this Mac" {
		t.Fatalf("a local cache reads as %v", got)
	}
	// A tool wrote the folder again, so the cache is here and the copy is out
	// of reach. The card must not say that the cache is on the drive.
	rewritten := exoCore.CacheItem{ID: "x", IsMigrated: true, IsActive: true, LinkState: exoCore.LinkRewritten}
	if got := stateOf(rewritten); got != stateRewritten || got.label(englishText) != "Rewritten" {
		t.Fatalf("a cache written again reads as %v", got)
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

// TestCountedKeepsThePlural keeps a count that names caches in each language.
// Chinese has one form, so both arguments carry it.
func TestCountedKeepsThePlural(t *testing.T) {
	if got := englishText.counted(1); got != "1 cache" {
		t.Fatalf("count(1) is %q", got)
	}
	if got := englishText.counted(0); got != "0 caches" {
		t.Fatalf("count(0) is %q", got)
	}
	if got := englishText.counted(2); got != "2 caches" {
		t.Fatalf("count(2) is %q", got)
	}
	if got := chineseText.counted(2); got != "2 个缓存" {
		t.Fatalf("Chinese count(2) is %q", got)
	}
}

// TestFamiliesGroupsTheCachesByToolchain keeps the rail honest: every cache
// lands in one group, the groups keep their order, and a toolchain the window
// does not know still shows up.
func TestFamiliesGroupsTheCachesByToolchain(t *testing.T) {
	items := []exoCore.CacheItem{
		{ID: "1", ToolName: "npm", SizeBytes: 10, IsActive: true},
		{ID: "2", ToolName: "pnpm", SizeBytes: 20, IsActive: true},
		{ID: "3", ToolName: "pip", SizeBytes: 40, IsActive: true},
		{ID: "4", ToolName: "cargo", SizeBytes: 80, IsMigrated: true, IsActive: true},
		{ID: "5", ToolName: "docker", SizeBytes: 160, IsActive: true},
		{ID: "6", ToolName: "something-new", SizeBytes: 320, IsActive: true},
		{ID: "7", ToolName: "xcode", SizeBytes: 640, IsActive: true},
		{ID: "8", ToolName: "maven", SizeBytes: 1280, IsActive: true},
		{ID: "9", ToolName: "dart", SizeBytes: 2560, IsActive: true},
		{ID: "10", ToolName: "android", SizeBytes: 5120, IsActive: true},
		{ID: "11", ToolName: "homebrew", SizeBytes: 10240, IsActive: true},
		{ID: "12", ToolName: "conda", SizeBytes: 20480, IsActive: true},
		{ID: "13", ToolName: "ruby", SizeBytes: 40960, IsActive: true},
	}

	got := families(items)
	want := []cacheFamily{familyNode, familyPython, familyRuby, familyRust, familyApple, familyJVM,
		familyMobile, familyHomebrew, familyDocker, familyOther}
	if len(got) != len(want) {
		t.Fatalf("families returned %d groups, want %d: %+v", len(got), len(want), got)
	}
	for i, family := range want {
		if got[i].Family != family {
			t.Fatalf("group %d is %q, want %q", i, got[i].Family, family)
		}
	}
	if got[0].Count != 2 || got[0].Local != 30 || got[0].Moved != 0 {
		t.Fatalf("Node is %+v, want 2 caches and 30 bytes on this Mac", got[0])
	}
	if got[3].Moved != 80 || got[3].Local != 0 {
		t.Fatalf("Rust is %+v, want its bytes on the drive", got[3])
	}

	if !hasFamily(items, familyOther) || hasFamily(items, familyGo) {
		t.Fatalf("hasFamily does not follow the items")
	}
	for tool, family := range map[string]cacheFamily{
		"xcode": familyApple, "cocoapods": familyApple, "swiftpm": familyApple,
		"maven": familyJVM, "gradle": familyJVM,
		"dart": familyMobile, "android": familyMobile,
		"homebrew": familyHomebrew, "conda": familyPython,
		"ruby": familyRuby, "composer": familyOther, "nuget": familyOther, "devtools": familyOther,
	} {
		if got := familyOf(tool); got != family {
			t.Fatalf("familyOf(%q) is %q, want %q", tool, got, family)
		}
	}
	if !inFamily(items[0], familyNode) || !inFamily(items[0], "") {
		t.Fatalf("inFamily drops a cache it should keep")
	}
	if inFamily(items[0], familyPython) {
		t.Fatalf("inFamily keeps a cache of another group")
	}
}

// TestInPlaceKeepsTheTwoPlaces keeps the place filter to the two answers a
// person looks for.
func TestInPlaceKeepsTheTwoPlaces(t *testing.T) {
	local := exoCore.CacheItem{ID: "a", IsActive: true}
	moved := exoCore.CacheItem{ID: "b", IsMigrated: true}
	for _, tc := range []struct {
		place string
		item  exoCore.CacheItem
		want  bool
	}{
		{placeAll, local, true}, {placeAll, moved, true},
		{placeLocal, local, true}, {placeLocal, moved, false},
		{placeMoved, moved, true}, {placeMoved, local, false},
	} {
		if got := inPlace(tc.item, tc.place); got != tc.want {
			t.Fatalf("inPlace(%q) is %v, want %v", tc.place, got, tc.want)
		}
	}
}

// TestMovedUnderCountsOneFolder keeps the drive gauge to the folder it shows: a
// second drive holds its own caches, and a folder whose name only starts the
// same way holds none of them.
func TestMovedUnderCountsOneFolder(t *testing.T) {
	here := exoCore.CacheItem{ID: "a", IsMigrated: true, SizeBytes: 100, DestinationPath: "/Volumes/SSD/exodrive/npm-cache"}
	other := exoCore.CacheItem{ID: "b", IsMigrated: true, SizeBytes: 200, DestinationPath: "/Volumes/Backup/exodrive/pip-cache"}
	sibling := exoCore.CacheItem{ID: "c", IsMigrated: true, SizeBytes: 400, DestinationPath: "/Volumes/SSD/exodrive-old/go-cache"}
	local := exoCore.CacheItem{ID: "d", IsActive: true, SizeBytes: 800}
	items := []exoCore.CacheItem{here, other, sibling, local}

	if got := movedUnder(items, "/Volumes/SSD/exodrive"); got != 100 {
		t.Fatalf("movedUnder counts %d bytes, want the 100 under the folder", got)
	}
	if got := movedUnder(items, "/Volumes/Backup/exodrive/"); got != 200 {
		t.Fatalf("movedUnder drops the drive a trailing separator names: %d", got)
	}
	if got := movedUnder(items, ""); got != 0 {
		t.Fatalf("movedUnder counts %d bytes for no folder", got)
	}
}
