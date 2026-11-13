package desktop

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/oneo-me/nsgo/core"
	nsdesktop "github.com/oneo-me/nsgo/desktop"
	"github.com/oneo-me/nsgo/ui"

	exoCore "exodrive/internal/core"
	"exodrive/internal/fsutil"
	"exodrive/internal/plugins"
)

// targetState is the chosen folder and the volume under it. Ready is true only
// after the engine checks the folder, so the window never starts a move into a
// folder that already failed the check. SameVolume is true when the folder sits
// on this Mac, where a move frees no space; the window warns and still moves.
type targetState struct {
	Path       string
	Free       int64
	Total      int64
	FS         string
	Ready      bool
	Problem    string
	SameVolume bool
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
	// Failures holds the reason a cache did not move, by cache ID.
	Failures *core.Signal[map[string]shownFailure]
	// FilterFamily holds the group the rail keeps, or "" for every group.
	FilterFamily *core.Signal[cacheFamily]
	// FilterPlace holds the place the list keeps: "" for every place, or
	// "local" and "moved" for the two places a person looks for.
	FilterPlace *core.Signal[string]

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

	ConfirmOpen  *core.Signal[bool]
	AdvancedOpen *core.Signal[bool]
	// WelcomeOpen shows the first-run card. It starts open until the person
	// takes the first step, and stays closed after that.
	WelcomeOpen *core.Signal[bool]
	// Update holds the newer release that the check found, or an empty one.
	Update *core.Signal[updateState]
	// ReportOpen shows the report dialog. ReportText holds the exact text that
	// the dialog shows, copies, and sends.
	ReportOpen *core.Signal[bool]
	ReportText *core.Signal[string]

	// Root is the window scope. Background work posts through it, because the
	// scope of a Dynamic branch is gone as soon as that branch re-renders.
	Root *core.Scope

	Engine *exoCore.Engine

	// Lang is the language of Text. A report names it, so a person knows which
	// words the window showed.
	Lang lang
	// Text holds every word the window writes, in the language it shows.
	Text text
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
	return window(cx, newState(cx, engine))
}

// window builds the shell over state. A test mounts it with its own state, so
// it can answer for the system picker the way the system does.
func window(cx *core.Scope, state *AppState) core.Node {
	cx.SetTheme(Theme())
	// The window title names the app in the title bar, the Dock, and the
	// window list. Without it the system shows the name of the binary.
	nsdesktop.SetTitle(cx, WindowTitle)
	triggerScan(state)
	// A record left by an earlier run names the cache that was changing when
	// the window closed. The scan clears the notice, so this comes after it.
	if n, ok := interruptedNotice(state.Text); ok {
		state.Notice.Set(n)
	}
	// The check runs on its own and never blocks the window.
	checkForUpdate(cx, state)

	return core.View{
		Style: core.Style{Direction: core.Column, Width: core.Pct(100), Height: core.Pct(100), Padding: core.All(20), Gap: 14},
		StyleFunc: func() core.Style {
			// One wash, from the accent toward nothing, so the page has a
			// light side. It carries the mark's first color and stops before
			// the list, where a tint would fight the state colors.
			colors := ui.ColorsOf(cx)
			return core.Style{Background: colors.Background(), Gradients: []core.Gradient{
				core.LinearGradient(135, ui.Alpha(colors.Accent(), 0.07), ui.Alpha(colors.Accent(), 0.02), ui.Alpha(colors.Accent(), 0)),
			}}
		},
		Children: []core.Node{
			renderHeader(cx, state),
			core.Show{When: func() bool { return state.WelcomeOpen.Get() }, Then: func(cx *core.Scope) core.Node { return renderWelcome(cx, state) }},
			renderDrives(cx, state),
			core.Dynamic(func(cx *core.Scope) core.Node { return renderAction(cx, state) }),
			renderListArea(cx, state),
			core.Show{When: func() bool { return state.AdvancedOpen.Get() }, Then: func(cx *core.Scope) core.Node { return renderAdvanced(cx, state) }},
			renderFooter(cx, state),
			renderDialogs(cx, state),
		},
	}
}

func newState(cx *core.Scope, engine *exoCore.Engine) *AppState {
	l := languageOf(cx)
	state := &AppState{
		Items:           core.NewSignalFunc(cx, []exoCore.CacheItem{}, nil),
		Selected:        core.NewSignalFunc(cx, map[string]bool{}, nil),
		FilterFamily:    core.NewSignal[cacheFamily](cx, ""),
		FilterPlace:     core.NewSignal(cx, ""),
		Failures:        core.NewSignalFunc(cx, map[string]shownFailure{}, nil),
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
		ConfirmOpen:     core.NewSignal(cx, false),
		AdvancedOpen:    core.NewSignal(cx, false),
		WelcomeOpen:     core.NewSignal(cx, !welcomeSeen()),
		Update:          core.NewSignal(cx, updateState{}),
		ReportOpen:      core.NewSignal(cx, false),
		ReportText:      core.NewSignal(cx, ""),
		Root:            cx,
		Engine:          engine,
		Lang:            l,
		Text:            texts(l),
	}
	return state
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

// visibleItems returns the caches the rail and the place filter keep. The
// window only shows these, and only these can move, so the list on screen is
// the list the move acts on.
func visibleItems(state *AppState) []exoCore.CacheItem {
	family := state.FilterFamily.Get()
	place := state.FilterPlace.Get()
	var out []exoCore.CacheItem
	for _, it := range state.Items.Get() {
		if inFamily(it, family) && inPlace(it, place) {
			out = append(out, it)
		}
	}
	return out
}

// setFamily keeps the caches of one group. It drops the selection, so a press
// on Move never moves a cache that the filter hides.
func setFamily(state *AppState, family cacheFamily) {
	state.FilterFamily.Set(family)
	state.Selected.Set(map[string]bool{})
}

// setPlace keeps the caches of one place, and drops the selection for the same
// reason as setFamily.
func setPlace(state *AppState, place string) {
	state.FilterPlace.Set(place)
	state.Selected.Set(map[string]bool{})
}

// openPicker asks the system for a folder, as Finder does. The system panel
// lists every connected drive and scrolls its own way, so no folder of this
// window's own has to.
func openPicker(state *AppState) {
	state.Root.PickDirectories(core.PickDirectoriesOptions{Limit: 1}, func(dirs []core.Directory, err error) {
		if err != nil {
			f := explain(err, actionChoose, state.Text)
			recordFailure(actionChoose, "", err)
			state.Notice.Set(notice{Kind: noticeDanger, Title: f.Title, Body: f.Body, Hint: f.Hint})
			return
		}
		if len(dirs) == 0 {
			return
		}
		if dirs[0].Path == "" {
			// A system that names no path cannot fill the field. Say so
			// rather than check a folder the window cannot name.
			state.Notice.Set(notice{Kind: noticeWarn, Title: state.Text.NoFolderPathTitle,
				Body: state.Text.NoFolderPathBody, Hint: state.Text.NoFolderPathHint})
			return
		}
		chooseFolder(state, dirs[0].Path)
	})
}

// chooseFolder takes a folder from the picker and checks it, exactly as a
// typed path is checked.
func chooseFolder(state *AppState, path string) {
	closeWelcome(state)
	state.TargetDir.Set(path)
	triggerTargetCheck(state)
}

// selectAll selects every cache on screen that can move, or clears the
// selection.
func selectAll(state *AppState, on bool) {
	next := map[string]bool{}
	if on {
		family := state.FilterFamily.GetUntracked()
		place := state.FilterPlace.GetUntracked()
		for _, it := range state.Items.GetUntracked() {
			if movable(it) && inFamily(it, family) && inPlace(it, place) {
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
				// A folder field runs a check on every edit, so a record here
				// would hold a line per keystroke. The move that follows
				// records the same reason once, and the field shows it now.
				state.Target.Set(targetState{Path: target, Problem: targetProblem(err, state.Text)})
				return
			}
			state.Target.Set(targetState{
				Path: target, Ready: true, FS: info.Filesystem,
				Free: info.FreeBytes, Total: info.TotalBytes,
				SameVolume: info.SameVolume,
			})
			// A checked folder is the first step of the first run.
			closeWelcome(state)
		})
	}()
}

func triggerScan(state *AppState) {
	if busy(state) {
		return
	}
	state.IsScanning.Set(true)
	state.Notice.Set(notice{})
	state.Status.Set(state.Text.CheckingCaches)

	go func() {
		ctx := context.Background()
		items, err := state.Engine.ScanAll(ctx)
		local := readDisk()
		recordFailure(actionScan, "", err)
		state.Root.Post(func() {
			state.IsScanning.Set(false)
			state.Local.Set(local)
			state.Failures.Set(map[string]shownFailure{})

			if items == nil {
				// A scan without a ledger cannot tell a moved cache from a
				// local one. Keep the list empty and say why.
				state.Items.Set(nil)
				state.Selected.Set(map[string]bool{})
				f := explain(err, actionMove, state.Text)
				f.Title = state.Text.CheckFailedTitle
				state.Notice.Set(notice{Kind: noticeDanger, Title: f.Title, Body: f.Body, Hint: f.Hint})
				state.Status.Set("")
				return
			}

			state.Items.Set(items)
			state.Selected.Set(map[string]bool{})
			// A group that lost its last cache would leave the rail with no
			// row to press, and an empty list with no way back.
			if family := state.FilterFamily.GetUntracked(); family != "" && !hasFamily(items, family) {
				state.FilterFamily.Set("")
			}

			if err != nil {
				state.Notice.Set(notice{Kind: noticeWarn,
					Title: state.Text.CheckPartialTitle,
					Body:  state.Text.CheckPartialBody,
					Hint:  state.Text.CheckPartialHint})
			}
			_, _, _, canMove := tally(items)
			state.Status.Set(fmt.Sprintf(state.Text.FoundCaches,
				state.Text.counted(len(items)), canMove))
		})
	}()
}

func triggerMigration(state *AppState) {
	if busy(state) {
		return
	}
	target := state.Target.Get()
	if !target.Ready {
		state.Notice.Set(notice{Kind: noticeDanger, Title: state.Text.ChooseFirstTitle,
			Body: state.Text.ChooseFirstBody, Hint: state.Text.ChooseFirstHint})
		return
	}
	toMigrate := selectedItems(state)
	if len(toMigrate) == 0 {
		return
	}
	t := state.Text

	state.IsMigrating.Set(true)
	state.CancelRequested.Set(false)
	state.ProgressPct.Set(0)
	state.MoveTotal.Set(len(toMigrate))
	state.MoveDone.Set(0)
	state.MoveCurrent.Set(toMigrate[0].Title)
	state.Notice.Set(notice{})
	state.Status.Set(fmt.Sprintf(state.Text.MovingStatus, state.Text.counted(len(toMigrate))))

	// The failures map is copied here, on the window thread, so the move
	// goroutine never reads a signal. A cache that runs again drops its old
	// failure.
	failures := make(map[string]shownFailure, 2)
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

			// The record names this cache until the move ends, so a window
			// that closes now leaves a reason for the next start.
			_ = exoCore.WriteInFlight(exoCore.InFlight{ID: it.ID, Title: it.Title, Action: actionMove, Target: target.Path})
			err := state.Engine.Migrate(ctx, it, target.Path, fsutil.MigrateProgress{
				Copy:   report(exoCore.PhaseCopy),
				Verify: report(exoCore.PhaseVerify),
			})
			_ = exoCore.ClearInFlight()
			if err != nil {
				failures[it.ID] = shownFailure{friendly: explain(err, actionMove, t), Action: actionMove, Item: it.ID, Cause: err}
				recordFailure(actionMove, it.ID, err)
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
				state.Status.Set(fmt.Sprintf(t.StoppedStatus, t.counted(moved)))
			case failed > 0:
				state.Status.Set(fmt.Sprintf(t.SomeMovedStatus, t.counted(moved), t.counted(failed)))
				title := fmt.Sprintf(t.SomeMovedTitleMany, failed)
				if failed == 1 {
					title = t.SomeMovedTitleOne
				}
				state.Notice.Set(notice{Kind: noticeWarn,
					Title: title,
					Body:  t.SomeMovedBody,
					Hint:  t.SomeMovedHint})
			default:
				state.Status.Set(fmt.Sprintf(t.MovedStatus, t.counted(moved), bytes(freed)))
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
		state.Status.Set(state.Text.NotInListStatus)
		return
	}

	state.IsCleaning.Set(true)
	state.Status.Set(fmt.Sprintf(state.Text.DeletingStatus, item.Title))
	state.Notice.Set(notice{})

	t := state.Text
	go func() {
		ctx := context.Background()
		_ = exoCore.WriteInFlight(exoCore.InFlight{ID: id, Title: item.Title, Action: actionDelete, Target: item.CurrentPath})
		err := state.Engine.CleanOrphan(item.CurrentPath)
		_ = exoCore.ClearInFlight()
		items, scanErr := state.Engine.ScanAll(ctx)
		local := readDisk()

		state.Root.Post(func() {
			state.IsCleaning.Set(false)
			if scanErr == nil {
				state.Items.Set(items)
			}
			state.Local.Set(local)
			if err != nil {
				f := shownFailure{friendly: explain(err, actionDelete, t), Action: actionDelete, Item: id, Cause: err}
				recordFailure(actionDelete, id, err)
				next := state.Failures.GetUntracked()
				next[id] = f
				state.Failures.Set(next)
				state.Notice.Set(notice{Kind: noticeDanger, Title: f.Title, Body: f.Body, Hint: f.Hint})
				return
			}
			state.Failures.Set(map[string]shownFailure{})
			state.Status.Set(fmt.Sprintf(t.DeletedStatus, item.Title, bytes(item.SizeBytes)))
		})
	}()
}

func triggerRollback(state *AppState, id string) {
	if busy(state) {
		return
	}
	state.IsRollingBack.Set(true)
	state.Status.Set(state.Text.RollingBackStatus)
	state.Notice.Set(notice{})

	t := state.Text
	item, _ := itemByID(state, id)
	go func() {
		ctx := context.Background()
		_ = exoCore.WriteInFlight(exoCore.InFlight{ID: id, Title: item.Title, Action: actionMoveBack, Target: item.DestinationPath})
		err := state.Engine.Rollback(ctx, id)
		_ = exoCore.ClearInFlight()
		items, scanErr := state.Engine.ScanAll(ctx)
		local := readDisk()

		state.Root.Post(func() {
			state.IsRollingBack.Set(false)
			if scanErr == nil {
				state.Items.Set(items)
			}
			state.Local.Set(local)
			if err != nil {
				f := shownFailure{friendly: explain(err, actionMoveBack, t), Action: actionMoveBack, Item: id, Cause: err}
				recordFailure(actionMoveBack, id, err)
				next := state.Failures.GetUntracked()
				next[id] = f
				state.Failures.Set(next)
				state.Notice.Set(notice{Kind: noticeDanger, Title: f.Title, Body: f.Body, Hint: f.Hint})
				return
			}
			state.Failures.Set(map[string]shownFailure{})
			state.Status.Set(t.RolledBackStatus)
		})
	}()
}
