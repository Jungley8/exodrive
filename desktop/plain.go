package desktop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	exoCore "exodrive/internal/core"
	internalUi "exodrive/internal/ui"
)

// Actions that can stop. The window names each one in its own words, so a
// message never mixes two of them up.
const (
	actionScan     = "scan"
	actionMove     = "move"
	actionMoveBack = "move back"
	actionDelete   = "delete"
	actionChoose   = "choose"
)

// friendly is a failure in the window's voice: what stopped, what stays safe,
// and the next step.
type friendly struct {
	Title string
	Body  string
	Hint  string
}

// explain turns an error into the window's voice. The rules read the error
// text that the engine and the providers write; an error that matches none of
// them keeps its own text, so the window never hides a reason.
func explain(err error, action string, t text) friendly {
	if err == nil {
		return friendly{}
	}
	raw := err.Error()
	f := friendly{Title: t.MoveStoppedTitle, Body: raw, Hint: t.RetryHint}
	switch action {
	case actionMoveBack:
		f.Title = t.MoveBackStopped
	case actionDelete:
		f.Title = t.DeleteStopped
	case actionChoose:
		f.Title = t.PickerFailedTitle
		f.Hint = t.PickerFailedHint
	}

	var target *exoCore.TargetError
	switch exoCore.FailureKind(err) {
	case exoCore.KindTarget:
		_ = errors.As(err, &target)
		f.Body, f.Hint = targetSentences(target, t)
	case exoCore.KindUnsupported:
		f.Title = t.DriveUnsupported
		f.Body = t.DriveUnsupportedWhy
		f.Hint = t.DriveUnsupportedFix
	case exoCore.KindSpace:
		f.Title = t.DriveFull
		f.Body = t.DriveFullWhy
		f.Hint = t.DriveFullFix
	case exoCore.KindRunning:
		f.Title = t.StillRunning
		f.Body = t.StillRunningWhy
		f.Hint = t.StillRunningFix
	case exoCore.KindSymlink:
		f.Title = t.AlreadyLink
		f.Body = t.AlreadyLinkWhy
		f.Hint = t.AlreadyLinkFix
	case exoCore.KindVerification:
		f.Title = t.CopyMismatch
		f.Body = t.CopyMismatchWhy
		f.Hint = t.CopyMismatchFix
	case exoCore.KindEnv:
		f.Title = t.ShellVariable
		f.Body = t.ShellVariableWhy
		name := raw[:strings.Index(raw, " is set to ")]
		name = name[strings.LastIndex(name, " ")+1:]
		f.Hint = fmt.Sprintf(t.ShellVariableFix, name)
	case exoCore.KindNoRecord:
		f.Title = t.NoRecord
		f.Body = t.NoRecordWhy
		f.Hint = t.NoRecordFix
	case exoCore.KindLedger:
		f.Title = t.LedgerUnreadable
		f.Body = t.LedgerUnreadableWhy
		f.Hint = t.LedgerUnreadableFix
	case exoCore.KindDiffers:
		f.Title = t.TwoCopies
		f.Body = t.TwoCopiesWhy
		f.Hint = t.TwoCopiesFix
	case exoCore.KindMissing:
		f.Title = t.FolderGone
		f.Body = t.FolderGoneWhy
		f.Hint = t.FolderGoneFix
	}
	return f
}

// targetSentences reads a folder error through its fields, so the window says
// why the folder failed in its own language.
func targetSentences(target *exoCore.TargetError, t text) (body, hint string) {
	if target.Volume != "" {
		return fmt.Sprintf(t.VolumeMissing, target.Volume), t.VolumeMissingFix
	}
	return fmt.Sprintf(t.FolderMissing, target.Path), t.FolderMissingFix
}

// targetProblem says why the window cannot use the chosen folder, in words a
// person can act on.
func targetProblem(err error, t text) string {
	if err == nil {
		return ""
	}
	var target *exoCore.TargetError
	if errors.As(err, &target) {
		body, _ := targetSentences(target, t)
		return body
	}
	raw := err.Error()
	switch {
	case strings.Contains(raw, "is not supported"):
		return t.DriveCannotHold
	case strings.Contains(raw, "does not exist"):
		return t.TargetMissingMessage
	}
	return raw
}

// shortPath hides the home folder behind a tilde, so a path fits a row.
func shortPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+string(filepath.Separator)) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

// familyLabel names a group. The toolchain names stay as they are, because a
// person reads them in the program's own settings.
func familyLabel(f cacheFamily, t text) string {
	if f == familyOther {
		return t.FamilyOther
	}
	return string(f)
}

// switchNote says what Exodrive changes so the program reaches the copy. A
// person needs to know if the change waits for a restart.
func switchNote(method exoCore.RedirectMethod, t text) string {
	switch method {
	case exoCore.RedirectNativeConfig:
		return t.SwitchNative
	case exoCore.RedirectSymlink:
		return t.SwitchLink
	case exoCore.RedirectManual:
		return t.SwitchManual
	}
	return ""
}

// cacheState is where a cache lives, as the row shows it.
type cacheState int

const (
	stateLocal cacheState = iota
	stateMoved
	stateBlocked
	stateUnused
)

func stateOf(it exoCore.CacheItem) cacheState {
	switch {
	case it.IsMigrated:
		return stateMoved
	case !it.IsActive:
		return stateUnused
	case it.BlockedReason != "":
		return stateBlocked
	}
	return stateLocal
}

func (s cacheState) label(t text) string {
	switch s {
	case stateMoved:
		return t.OnTheDrive
	case stateBlocked:
		return t.CannotMove
	case stateUnused:
		return t.Unused
	}
	return t.OnThisMac
}

// movable reports whether Exodrive can move the cache now.
func movable(it exoCore.CacheItem) bool {
	return !it.IsMigrated && it.IsActive && it.BlockedReason == ""
}

// tally sums the window's numbers: bytes by place, and how many caches can
// move.
func tally(items []exoCore.CacheItem) (local, moved, unused int64, canMove int) {
	for _, it := range items {
		switch stateOf(it) {
		case stateMoved:
			moved += it.SizeBytes
		case stateUnused:
			unused += it.SizeBytes
		default:
			local += it.SizeBytes
			if movable(it) {
				canMove++
			}
		}
	}
	return local, moved, unused, canMove
}

// movedUnder is the bytes of the caches that sit under root. A window that
// holds one folder at a time counts only what that folder holds, so caches on
// another drive never add to it.
func movedUnder(items []exoCore.CacheItem, root string) int64 {
	if root == "" {
		return 0
	}
	prefix := strings.TrimSuffix(root, string(filepath.Separator)) + string(filepath.Separator)
	var sum int64
	for _, it := range items {
		if stateOf(it) == stateMoved && strings.HasPrefix(it.DestinationPath, prefix) {
			sum += it.SizeBytes
		}
	}
	return sum
}

// bytes is a formatted size, for the window's own labels.
func bytes(n int64) string { return internalUi.FormatBytes(n) }

// cacheFamily groups a toolchain under the name a person knows. The rail lists
// the groups, and a press on one keeps only that group's caches in the list.
type cacheFamily string

const (
	familyNode   cacheFamily = "Node"
	familyPython cacheFamily = "Python"
	familyRust   cacheFamily = "Rust"
	familyGo     cacheFamily = "Go"
	familyDocker cacheFamily = "Docker"
	familyOther  cacheFamily = "Other"
)

// familyOrder is the order the rail lists the groups in. A group with no cache
// does not appear.
var familyOrder = []cacheFamily{familyNode, familyPython, familyRust, familyGo, familyDocker, familyOther}

// familyOf maps a toolchain to its group. A toolchain the window does not know
// falls in Other, so its caches stay visible and cannot hide.
func familyOf(tool string) cacheFamily {
	switch strings.ToLower(strings.TrimSpace(tool)) {
	case "npm", "pnpm", "yarn", "bun":
		return familyNode
	case "pip", "uv":
		return familyPython
	case "cargo":
		return familyRust
	case "go", "golang":
		return familyGo
	case "docker", "orbstack":
		return familyDocker
	}
	return familyOther
}

// familyStat is one group's count and bytes in the rail.
type familyStat struct {
	Family cacheFamily
	Count  int
	Local  int64
	Moved  int64
}

// families sums the caches by group, in the order the rail lists them.
func families(items []exoCore.CacheItem) []familyStat {
	stats := make(map[cacheFamily]*familyStat, len(familyOrder))
	for _, it := range items {
		family := familyOf(it.ToolName)
		stat, ok := stats[family]
		if !ok {
			stat = &familyStat{Family: family}
			stats[family] = stat
		}
		stat.Count++
		switch stateOf(it) {
		case stateMoved:
			stat.Moved += it.SizeBytes
		default:
			stat.Local += it.SizeBytes
		}
	}

	out := make([]familyStat, 0, len(stats))
	for _, family := range familyOrder {
		if stat, ok := stats[family]; ok {
			out = append(out, *stat)
		}
	}
	return out
}

// hasFamily reports whether a group holds a cache in items.
func hasFamily(items []exoCore.CacheItem, family cacheFamily) bool {
	for _, it := range items {
		if familyOf(it.ToolName) == family {
			return true
		}
	}
	return false
}

// inFamily reports whether a cache belongs to family. An empty family holds
// every cache.
func inFamily(it exoCore.CacheItem, family cacheFamily) bool {
	return family == "" || familyOf(it.ToolName) == family
}

// The places the list filter keeps. An empty place keeps every place.
const (
	placeAll   = ""
	placeLocal = "local"
	placeMoved = "moved"
)

// inPlace reports whether a cache sits in the place the filter keeps.
func inPlace(it exoCore.CacheItem, place string) bool {
	switch place {
	case placeLocal:
		return stateOf(it) != stateMoved
	case placeMoved:
		return stateOf(it) == stateMoved
	}
	return true
}
