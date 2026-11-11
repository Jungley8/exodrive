package desktop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/ui"
	"github.com/oneo-me/nsgo/ui/filepicker"

	exoCore "exodrive/internal/core"
	"exodrive/internal/fsutil"
	"exodrive/internal/plugins"
)

// targetState is the chosen folder and the volume under it. Ready is true only
// after the engine checks the folder, so the window never starts a move into a
// folder that already failed the check.
type targetState struct {
	Path    string
	Free    int64
	Total   int64
	FS      string
	Ready   bool
	Problem string
}

// diskState is the free space of this Mac.
type diskState struct {
	Free  int64
	Total int64
	Known bool
}

// noticeKind picks the color and the icon of the banner above the list.
type noticeKind int

const (
	noticeNone noticeKind = iota
	noticeInfo
	noticeWarn
	noticeDanger
)

// notice is the one banner the window shows. A zero notice shows nothing.
type notice struct {
	Kind  noticeKind
	Title string
	Body  string
	Hint  string
}

func (n notice) shown() bool { return n.Kind != noticeNone && n.Title != "" }

// AppState holds everything the window shows. The signals live for the life of
// the window; each view reads them through Dynamic or a ...Func field, so a
// change repaints only the part that shows it.
type AppState struct {
	Items    *core.Signal[[]exoCore.CacheItem]
	Selected *core.Signal[map[string]bool]
	// OpenRows holds the caches whose details the user opened.
	OpenRows *core.Signal[map[string]bool]
	// Failures holds the reason a cache did not move, by cache ID.
	Failures *core.Signal[map[string]friendly]

	TargetDir *core.Signal[string]
	Target    *core.Signal[targetState]
	Local     *core.Signal[diskState]

	Notice *core.Signal[notice]
	Status *core.Signal[string]

	IsScanning  *core.Signal[bool]
	IsMigrating *core.Signal[bool]
	ProgressPct *core.Signal[float32]

	// IsRollingBack blocks a second move back and the other long actions.
	IsRollingBack *core.Signal[bool]
	// RollbackTarget holds the cache ID that waits for confirmation, or "".
	RollbackTarget *core.Signal[string]

	// IsCleaning blocks a second delete and the other long actions.
	IsCleaning *core.Signal[bool]
	// CleanTarget holds the unused cache ID that waits for confirmation, or "".
	CleanTarget *core.Signal[string]

	// MoveCurrent is the title of the cache that the window moves now.
	MoveCurrent *core.Signal[string]
	// MovePhase names the step that runs now: copy or check.
	MovePhase *core.Signal[exoCore.MovePhase]
	// MoveDone counts the finished moves of this run; MoveTotal is its size.
	MoveDone  *core.Signal[int]
	MoveTotal *core.Signal[int]
	// CancelRequested asks the running move to stop after the current cache.
	CancelRequested *core.Signal[bool]

	PickerOpen   *core.Signal[bool]
	ConfirmOpen  *core.Signal[bool]
	AdvancedOpen *core.Signal[bool]

	Picker       *filepicker.State
	PickerDevice string

	// Root is the window scope. Background work posts through it, because the
	// scope of a Dynamic branch is gone as soon as that branch re-renders.
	Root *core.Scope

	Engine *exoCore.Engine
}

// NewEngine builds the engine over every registered toolchain provider.
func NewEngine() *exoCore.Engine {
	reg := plugins.NewRegistry()
	accessors := make([]exoCore.ProviderAccessor, 0, len(reg.All()))
	for _, p := range reg.All() {
		accessors = append(accessors, p)
	}
	return exoCore.NewEngine(accessors...)
}

// NewMain builds the window for engine. Tests use it to mount the window with
// a stand-in engine.
func NewMain(engine *exoCore.Engine) func(*core.Scope) core.Node {
	return func(cx *core.Scope) core.Node { return root(cx, engine) }
}

// Root is the window the binary opens.
func Root(cx *core.Scope) core.Node { return root(cx, NewEngine()) }

func root(cx *core.Scope, engine *exoCore.Engine) core.Node {
	cx.SetTheme(Theme())
	state := newState(cx, engine)
	triggerScan(state)

	return core.View{
		Style:     core.Style{Direction: core.Column, Width: core.Pct(100), Height: core.Pct(100), Padding: core.All(20), Gap: 14},
		StyleFunc: func() core.Style { return core.Style{Background: ui.ColorsOf(cx).Background()} },
		Children: []core.Node{
			renderHeader(cx, state),
			renderDrives(cx, state),
			core.Dynamic(func(cx *core.Scope) core.Node { return renderAction(cx, state) }),
			renderListHeader(cx, state),
			core.Dynamic(func(cx *core.Scope) core.Node { return renderList(cx, state) }),
			core.Show{When: func() bool { return state.AdvancedOpen.Get() }, Then: func(cx *core.Scope) core.Node { return renderAdvanced(cx, state) }},
			renderFooter(cx, state),
			renderPicker(state),
			renderDialogs(cx, state),
		},
	}
}

func newState(cx *core.Scope, engine *exoCore.Engine) *AppState {
	state := &AppState{
		Items:           core.NewSignalFunc(cx, []exoCore.CacheItem{}, nil),
		Selected:        core.NewSignalFunc(cx, map[string]bool{}, nil),
		OpenRows:        core.NewSignalFunc(cx, map[string]bool{}, nil),
		Failures:        core.NewSignalFunc(cx, map[string]friendly{}, nil),
		TargetDir:       core.NewSignal(cx, ""),
		Target:          core.NewSignal(cx, targetState{}),
		Local:           core.NewSignal(cx, diskState{}),
		Notice:          core.NewSignal(cx, notice{}),
		Status:          core.NewSignal(cx, ""),
		IsScanning:      core.NewSignal(cx, false),
		IsMigrating:     core.NewSignal(cx, false),
		ProgressPct:     core.NewSignal[float32](cx, 0),
		IsRollingBack:   core.NewSignal(cx, false),
		RollbackTarget:  core.NewSignal(cx, ""),
		IsCleaning:      core.NewSignal(cx, false),
		CleanTarget:     core.NewSignal(cx, ""),
		MoveCurrent:     core.NewSignal(cx, ""),
		MovePhase:       core.NewSignal(cx, exoCore.PhaseCopy),
		MoveDone:        core.NewSignal(cx, 0),
		MoveTotal:       core.NewSignal(cx, 0),
		CancelRequested: core.NewSignal(cx, false),
		PickerOpen:      core.NewSignal(cx, false),
		ConfirmOpen:     core.NewSignal(cx, false),
		AdvancedOpen:    core.NewSignal(cx, false),
		Picker:          filepicker.NewState(cx, filepicker.List, false, 0),
		Root:            cx,
		Engine:          engine,
	}
	state.Picker.Load = loadFolder
	state.PickerDevice, _ = os.Hostname()
	return state
}

// loadFolder lists a directory for the folder picker.
func loadFolder(path string, done func(filepicker.Folder, error)) {
	if strings.TrimSpace(path) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			done(filepicker.Folder{}, err)
			return
		}
		path = home
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		done(filepicker.Folder{}, err)
		return
	}

	folder := filepicker.Folder{Path: path}
	for _, entry := range entries {
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		folder.Entries = append(folder.Entries, filepicker.Entry{
			Name:     entry.Name(),
			Path:     filepath.Join(path, entry.Name()),
			Dir:      entry.IsDir(),
			Size:     info.Size(),
			Modified: info.ModTime(),
			Hidden:   strings.HasPrefix(entry.Name(), "."),
		})
	}
	done(folder, nil)
}

// busy reports whether a long action runs. A second action waits for it.
func busy(state *AppState) bool {
	return state.IsScanning.Get() || state.IsMigrating.Get() || state.IsRollingBack.Get() || state.IsCleaning.Get()
}

func toggleSelected(state *AppState, id string) {
	next := make(map[string]bool, len(state.Selected.GetUntracked()))
	for key, value := range state.Selected.GetUntracked() {
		next[key] = value
	}
	next[id] = !next[id]
	state.Selected.Set(next)
}

func toggleOpen(state *AppState, id string) {
	next := make(map[string]bool, len(state.OpenRows.GetUntracked()))
	for key, value := range state.OpenRows.GetUntracked() {
		next[key] = value
	}
	next[id] = !next[id]
	state.OpenRows.Set(next)
}

// selectAll selects every cache that can move, or clears the selection.
func selectAll(state *AppState, on bool) {
	next := map[string]bool{}
	if on {
		for _, it := range state.Items.Get() {
			if movable(it) {
				next[it.ID] = true
			}
		}
	}
	state.Selected.Set(next)
}

// selectedItems returns the selected caches that can move now.
func selectedItems(state *AppState) []exoCore.CacheItem {
	selected := state.Selected.Get()
	var out []exoCore.CacheItem
	for _, it := range state.Items.Get() {
		if selected[it.ID] && movable(it) {
			out = append(out, it)
		}
	}
	return out
}

// itemByID returns the cache with this ID, and false when the scan no longer
// holds it.
func itemByID(state *AppState, id string) (exoCore.CacheItem, bool) {
	for _, it := range state.Items.GetUntracked() {
		if it.ID == id {
			return it, true
		}
	}
	return exoCore.CacheItem{}, false
}

// readDisk reports the free space of this Mac.
func readDisk() diskState {
	home, err := os.UserHomeDir()
	if err != nil {
		return diskState{}
	}
	free, total, _, err := fsutil.GetDiskUsage(home)
	if err != nil {
		return diskState{}
	}
	return diskState{Free: free, Total: total, Known: true}
}

// triggerTargetCheck reports the volume under the target folder, or the reason
// it fails. It runs after every edit, and it drops a result that a newer edit
// made stale.
func triggerTargetCheck(state *AppState) {
	target := strings.TrimSpace(state.TargetDir.Get())
	if target == "" {
		state.Target.Set(targetState{})
		return
	}

	go func() {
		info, err := state.Engine.CheckTarget(target)
		state.Root.Post(func() {
			if strings.TrimSpace(state.TargetDir.Get()) != target {
				return
			}
			if err != nil {
				state.Target.Set(targetState{Path: target, Problem: targetProblem(err)})
				return
			}
			state.Target.Set(targetState{
				Path: target, Ready: true, FS: info.Filesystem,
				Free: info.FreeBytes, Total: info.TotalBytes,
			})
		})
	}()
}

func triggerScan(state *AppState) {
	if busy(state) {
		return
	}
	state.IsScanning.Set(true)
	state.Notice.Set(notice{})
	state.Status.Set("Checking this Mac for caches...")

	go func() {
		ctx := context.Background()
		items, err := state.Engine.ScanAll(ctx)
		local := readDisk()
		state.Root.Post(func() {
			state.IsScanning.Set(false)
			state.Local.Set(local)
			state.Failures.Set(map[string]friendly{})
			state.OpenRows.Set(map[string]bool{})

			if items == nil {
				// A scan without a ledger cannot tell a moved cache from a
				// local one. Keep the list empty and say why.
				state.Items.Set(nil)
				state.Selected.Set(map[string]bool{})
				f := explain(err, actionMove)
				f.Title = "Exodrive cannot check this Mac"
				state.Notice.Set(notice{Kind: noticeDanger, Title: f.Title, Body: f.Body, Hint: f.Hint})
				state.Status.Set("")
				return
			}

			state.Items.Set(items)
			state.Selected.Set(map[string]bool{})

			if err != nil {
				state.Notice.Set(notice{Kind: noticeWarn,
					Title: "Part of the check did not run",
					Body:  "Exodrive could not read every program, so the list may be short.",
					Hint:  "Quit the programs below and scan again."})
			}
			_, _, _, canMove := tally(items)
			state.Status.Set(fmt.Sprintf("Exodrive found %s. %d can move.",
				count(len(items), "cache", "caches"), canMove))
		})
	}()
}

func triggerMigration(state *AppState) {
	if busy(state) {
		return
	}
	target := state.Target.Get()
	if !target.Ready {
		state.Notice.Set(notice{Kind: noticeDanger, Title: "Choose a folder first", Body: "Exodrive needs a checked folder on the external drive.", Hint: "Choose a folder, then try again."})
		return
	}
	toMigrate := selectedItems(state)
	if len(toMigrate) == 0 {
		return
	}

	state.IsMigrating.Set(true)
	state.CancelRequested.Set(false)
	state.ProgressPct.Set(0)
	state.MoveTotal.Set(len(toMigrate))
	state.MoveDone.Set(0)
	state.MoveCurrent.Set(toMigrate[0].Title)
	state.Notice.Set(notice{})
	state.Status.Set(fmt.Sprintf("Moving %s...", count(len(toMigrate), "cache", "caches")))

	// The failures map is copied here, on the window thread, so the move
	// goroutine never reads a signal. A cache that runs again drops its old
	// failure.
	failures := make(map[string]friendly, 2)
	for key, value := range state.Failures.GetUntracked() {
		failures[key] = value
	}
	for _, it := range toMigrate {
		delete(failures, it.ID)
	}

	go func() {
		ctx := context.Background()
		moved, failed := 0, 0
		var freed int64
		stopped := false

		for index, it := range toMigrate {
			if state.CancelRequested.GetUntracked() {
				stopped = true
				break
			}
			state.Root.Post(func() {
				state.MoveCurrent.Set(it.Title)
				state.MoveDone.Set(index)
				state.ProgressPct.Set(0)
				state.MovePhase.Set(exoCore.PhaseCopy)
			})

			// Report 1% steps: a large file copies in 1 MiB chunks and would
			// otherwise post thousands of updates.
			lastPct := float32(-1)
			lastPhase := exoCore.PhaseCopy
			report := func(phase exoCore.MovePhase) fsutil.CopyProgressCallback {
				return func(done, total int64) {
					if total <= 0 {
						return
					}
					pct := float32(done) / float32(total)
					if pct > 1 {
						pct = 1
					}
					// Each step starts at 0, so its first percent shows even
					// when the copy ended at 100.
					if phase != lastPhase {
						lastPhase = phase
						lastPct = -1
					}
					if pct-lastPct < 0.01 && pct < 1 {
						return
					}
					lastPct = pct
					state.Root.Post(func() {
						state.MovePhase.Set(phase)
						state.ProgressPct.Set(pct)
					})
				}
			}

			err := state.Engine.Migrate(ctx, it, target.Path, fsutil.MigrateProgress{
				Copy:   report(exoCore.PhaseCopy),
				Verify: report(exoCore.PhaseVerify),
			})
			if err != nil {
				failures[it.ID] = explain(err, actionMove)
				failed++
				continue
			}
			moved++
			freed += it.SizeBytes
		}

		items, scanErr := state.Engine.ScanAll(ctx)
		local := readDisk()
		state.Root.Post(func() {
			if scanErr == nil {
				state.Items.Set(items)
			}
			state.Failures.Set(failures)
			state.Selected.Set(map[string]bool{})
			state.Local.Set(local)
			state.IsMigrating.Set(false)
			state.CancelRequested.Set(false)
			state.ProgressPct.Set(0)
			state.MoveTotal.Set(0)
			state.MoveDone.Set(0)
			state.MoveCurrent.Set("")

			switch {
			case stopped:
				state.Status.Set(fmt.Sprintf("Stopped. %s moved. The other caches stay on this Mac.",
					count(moved, "cache", "caches")))
			case failed > 0:
				state.Status.Set(fmt.Sprintf("%s moved. %s did not move.", count(moved, "cache", "caches"), count(failed, "cache", "caches")))
				state.Notice.Set(notice{Kind: noticeWarn,
					Title: count(failed, "cache did not move", "caches did not move"),
					Body:  "Each reason sits on its cache row below. Exodrive deleted nothing.",
					Hint:  "Open a cache to read the reason, then try again."})
			default:
				state.Status.Set(fmt.Sprintf("Moved %s. %s is now free on this Mac.", count(moved, "cache", "caches"), bytes(freed)))
			}
		})
	}()
}

func triggerClean(state *AppState, id string) {
	if busy(state) {
		return
	}
	item, found := itemByID(state, id)
	if !found {
		state.Status.Set("This cache is not in the list anymore. Run a scan.")
		return
	}

	state.IsCleaning.Set(true)
	state.Status.Set(fmt.Sprintf("Deleting %s...", item.Title))
	state.Notice.Set(notice{})

	go func() {
		ctx := context.Background()
		err := state.Engine.CleanOrphan(item.CurrentPath)
		items, scanErr := state.Engine.ScanAll(ctx)
		local := readDisk()

		state.Root.Post(func() {
			state.IsCleaning.Set(false)
			if scanErr == nil {
				state.Items.Set(items)
			}
			state.Local.Set(local)
			if err != nil {
				f := explain(err, actionDelete)
				next := state.Failures.GetUntracked()
				next[id] = f
				state.Failures.Set(next)
				state.OpenRows.Set(withOpen(state.OpenRows.GetUntracked(), id))
				state.Notice.Set(notice{Kind: noticeDanger, Title: f.Title, Body: f.Body, Hint: f.Hint})
				return
			}
			state.Failures.Set(map[string]friendly{})
			state.Status.Set(fmt.Sprintf("Deleted %s. %s is now free.", item.Title, bytes(item.SizeBytes)))
		})
	}()
}

func triggerRollback(state *AppState, id string) {
	if busy(state) {
		return
	}
	state.IsRollingBack.Set(true)
	state.Status.Set("Moving the cache back to this Mac...")
	state.Notice.Set(notice{})

	go func() {
		ctx := context.Background()
		err := state.Engine.Rollback(ctx, id)
		items, scanErr := state.Engine.ScanAll(ctx)
		local := readDisk()

		state.Root.Post(func() {
			state.IsRollingBack.Set(false)
			if scanErr == nil {
				state.Items.Set(items)
			}
			state.Local.Set(local)
			if err != nil {
				f := explain(err, actionMoveBack)
				next := state.Failures.GetUntracked()
				next[id] = f
				state.Failures.Set(next)
				state.OpenRows.Set(withOpen(state.OpenRows.GetUntracked(), id))
				state.Notice.Set(notice{Kind: noticeDanger, Title: f.Title, Body: f.Body, Hint: f.Hint})
				return
			}
			state.Failures.Set(map[string]friendly{})
			state.Status.Set("The cache is back on this Mac. Exodrive removed the record of the move.")
		})
	}()
}

// withOpen returns the open rows with id added.
func withOpen(open map[string]bool, id string) map[string]bool {
	next := make(map[string]bool, len(open)+1)
	for key, value := range open {
		next[key] = value
	}
	next[id] = true
	return next
}
