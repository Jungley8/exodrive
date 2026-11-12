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
	actionMove     = "move"
	actionMoveBack = "move back"
	actionDelete   = "delete"
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
func explain(err error, action string) friendly {
	if err == nil {
		return friendly{}
	}
	raw := err.Error()
	f := friendly{Title: "The move stopped", Body: raw, Hint: "Exodrive deleted nothing. Try again."}
	switch action {
	case actionMoveBack:
		f.Title = "The move back stopped"
	case actionDelete:
		f.Title = "The delete stopped"
	}

	var target *exoCore.TargetError
	switch {
	case errors.As(err, &target):
		f.Body, f.Hint = splitSentences(target.Message)
	case strings.Contains(raw, "is not supported"), strings.Contains(raw, "incompatible filesystem"):
		f.Title = "Exodrive cannot use this drive"
		f.Body = "The drive uses a format that Exodrive cannot use for caches."
		f.Hint = "Choose a folder on another drive. Exodrive supports APFS, HFS+, ext2, ext3, ext4, btrfs, zfs, xfs, f2fs, and bcachefs."
	case strings.Contains(raw, "insufficient space"):
		f.Title = "The drive is full"
		f.Body = "The drive does not have enough free space for this cache."
		f.Hint = "Free space on the drive, or move fewer caches at a time."
	case strings.Contains(raw, "is running"):
		f.Title = "A program still uses this cache"
		f.Body = "A program that writes to this cache is open."
		f.Hint = "Quit the program, then try again."
	case strings.Contains(raw, "is already a symlink"):
		f.Title = "This folder already links elsewhere"
		f.Body = "The folder points to a copy that Exodrive did not record."
		f.Hint = "Run a scan. If the row keeps this message, the cache needs no move."
	case strings.Contains(raw, "verification failed"):
		f.Title = "The copy did not match"
		f.Body = "The copy on the drive differs from the original."
		f.Hint = "Exodrive deleted nothing. Try again. If it fails again, check the drive."
	case strings.Contains(raw, " is set to "):
		f.Title = "Your shell sets the cache path"
		f.Body = "A variable in your shell overrides the setting that Exodrive changes."
		name := raw[:strings.Index(raw, " is set to ")]
		name = name[strings.LastIndex(name, " ")+1:]
		f.Hint = fmt.Sprintf("Remove %s from your shell, then try again.", name)
	case strings.Contains(raw, "no record"), strings.Contains(raw, "no migration record"):
		f.Title = "No record of this move"
		f.Body = "Exodrive cannot find the move that put this cache on the drive."
		f.Hint = "Run a scan, then choose the cache again."
	case strings.Contains(raw, "ledger"), strings.Contains(raw, "manifest"):
		f.Title = "Exodrive cannot read its move records"
		f.Body = "The record file is missing or unreadable."
		f.Hint = "Check that the record file is readable, then scan again."
	case strings.Contains(raw, "exists and differs"):
		f.Title = "Two copies hold different data"
		f.Body = "The folder on this Mac and the copy on the drive are not the same."
		f.Hint = "Both copies are safe. Exodrive stops so it cannot pick one for you."
	case strings.Contains(raw, "does not exist"), strings.Contains(raw, "not available"):
		f.Title = "The folder is gone"
		f.Body = "The folder that Exodrive needs is not there."
		f.Hint = "Choose the folder again, then try again."
	}
	return f
}

// splitSentences splits a two-sentence message into the statement and the next
// step, so each sits on its own line.
func splitSentences(message string) (body, hint string) {
	if i := strings.Index(message, ". "); i >= 0 {
		return message[:i+1], message[i+2:]
	}
	return message, ""
}

// targetProblem says why the window cannot use the chosen folder, in words a
// person can act on.
func targetProblem(err error) string {
	if err == nil {
		return ""
	}
	var target *exoCore.TargetError
	if errors.As(err, &target) {
		return target.Message
	}
	raw := err.Error()
	switch {
	case strings.Contains(raw, "is not supported"):
		return "This drive cannot hold caches. Choose a folder on an APFS, HFS+, ext2, ext3, ext4, btrfs, zfs, xfs, f2fs, or bcachefs drive."
	case strings.Contains(raw, "does not exist"):
		return "This folder does not exist. Choose a folder on the drive."
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

// switchNote says what Exodrive changes so the program reaches the copy. A
// person needs to know if the change waits for a restart.
func switchNote(method exoCore.RedirectMethod) string {
	switch method {
	case exoCore.RedirectNativeConfig:
		return "Exodrive changes this program's setting. The program reads it at its next start."
	case exoCore.RedirectSymlink:
		return "Exodrive links the old folder to the copy. The program sees no change."
	case exoCore.RedirectManual:
		return "This cache is not in use. Delete it to free the space."
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

func (s cacheState) label() string {
	switch s {
	case stateMoved:
		return "On the drive"
	case stateBlocked:
		return "Cannot move"
	case stateUnused:
		return "Unused"
	}
	return "On this Mac"
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

// count names a number of caches, with the right plural.
func count(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
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
