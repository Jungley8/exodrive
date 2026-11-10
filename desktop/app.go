package desktop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/icons/lucide"
	"github.com/oneo-me/nsgo/ui"
	"github.com/oneo-me/nsgo/ui/filepicker"

	exoCore "exodrive/internal/core"
	"exodrive/internal/plugins"
	internalUi "exodrive/internal/ui"
)

// AppState holds everything the window shows. The signals live for the life of
// the window; each view reads them through Dynamic or a ...Func field, so a
// change repaints only the part that shows it.
type AppState struct {
	Items       *core.Signal[[]exoCore.CacheItem]
	Selected    *core.Signal[map[string]bool]
	TargetDir   *core.Signal[string]
	StatusMsg   *core.Signal[string]
	IsScanning  *core.Signal[bool]
	IsMigrating *core.Signal[bool]
	ProgressPct *core.Signal[float32]

	// IsRollingBack blocks a second rollback and the other long actions.
	IsRollingBack *core.Signal[bool]
	// RollbackTarget holds the item ID that waits for confirmation, or "".
	RollbackTarget *core.Signal[string]

	// IsCleaning blocks a second delete and the other long actions.
	IsCleaning *core.Signal[bool]
	// CleanTarget holds the abandoned item ID that waits for confirmation, or "".
	CleanTarget *core.Signal[string]

	// TargetInfo holds the volume summary, or the reason the folder fails.
	TargetInfo *core.Signal[string]
	// MoveCurrent is the title of the cache that the window moves now.
	MoveCurrent *core.Signal[string]
	// MoveDone counts the finished moves of this run; MoveTotal is its size.
	MoveDone  *core.Signal[int]
	MoveTotal *core.Signal[int]
	// CancelRequested asks the running move to stop after the current cache.
	CancelRequested *core.Signal[bool]

	PickerOpen   *core.Signal[bool]
	ConfirmOpen  *core.Signal[bool]
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
	state := &AppState{
		Items:           core.NewSignalFunc(cx, []exoCore.CacheItem{}, nil),
		Selected:        core.NewSignalFunc(cx, map[string]bool{}, nil),
		TargetDir:       core.NewSignal(cx, ""),
		StatusMsg:       core.NewSignal(cx, "Ready. Choose a target folder, then scan."),
		IsScanning:      core.NewSignal(cx, false),
		IsMigrating:     core.NewSignal(cx, false),
		ProgressPct:     core.NewSignal[float32](cx, 0),
		PickerOpen:      core.NewSignal(cx, false),
		ConfirmOpen:     core.NewSignal(cx, false),
		IsRollingBack:   core.NewSignal(cx, false),
		RollbackTarget:  core.NewSignal(cx, ""),
		IsCleaning:      core.NewSignal(cx, false),
		CleanTarget:     core.NewSignal(cx, ""),
		TargetInfo:      core.NewSignal(cx, ""),
		MoveCurrent:     core.NewSignal(cx, ""),
		MoveDone:        core.NewSignal(cx, 0),
		MoveTotal:       core.NewSignal(cx, 0),
		CancelRequested: core.NewSignal(cx, false),
		Picker:          filepicker.NewState(cx, filepicker.List, false, 0),
		Root:            cx,
		Engine:          engine,
	}
	state.Picker.Load = loadFolder
	state.PickerDevice, _ = os.Hostname()

	triggerScan(state)

	return core.View{
		Class: "flex-col w-full h-full bg-slate-50 dark:bg-zinc-900 p-5 gap-4",
		Children: []core.Node{
			renderHeader(cx, state),
			core.Dynamic(func(cx *core.Scope) core.Node { return renderStats(cx, state) }),
			core.Dynamic(func(cx *core.Scope) core.Node { return renderToolbar(cx, state) }),
			renderTargetRow(cx, state),
			renderGuide(state),
			core.Dynamic(func(cx *core.Scope) core.Node { return renderItemList(cx, state) }),
			renderFooter(cx, state),
			renderPicker(state),
			renderConfirm(cx, state),
			renderRollbackConfirm(state),
			renderCleanConfirm(state),
		},
	}
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

func renderHeader(cx *core.Scope, state *AppState) core.Node {
	return core.View{
		Class: "flex-row justify-between items-center pb-2 border-b border-slate-200 dark:border-zinc-800",
		Children: []core.Node{
			core.View{
				Class: "flex-col",
				Children: []core.Node{
					core.Text{
						Class:   "text-xl font-bold text-slate-800 dark:text-zinc-100",
						Content: "Exodrive",
					},
					core.Text{
						Class:   "text-xs text-slate-500 dark:text-zinc-400",
						Content: "Move developer caches to external storage and reclaim local disk space",
					},
				},
			},
			core.Text{
				Class:       "text-xs text-slate-500 dark:text-zinc-400 italic",
				ContentFunc: func() string { return state.StatusMsg.Get() },
			},
		},
	}
}

func renderStats(cx *core.Scope, state *AppState) core.Node {
	var localBytes, migratedBytes, orphanBytes int64
	for _, it := range state.Items.Get() {
		switch {
		case it.Category == exoCore.CategoryOrphan:
			orphanBytes += it.SizeBytes
		case it.IsMigrated:
			migratedBytes += it.SizeBytes
		default:
			localBytes += it.SizeBytes
		}
	}

	return core.View{
		Class: "flex-row gap-3 w-full",
		Children: []core.Node{
			statCard("Local SSD Cache", internalUi.FormatBytes(localBytes), "text-amber-600 dark:text-amber-400"),
			statCard("External Migrated", internalUi.FormatBytes(migratedBytes), "text-emerald-600 dark:text-emerald-400"),
			statCard("Abandoned Caches", internalUi.FormatBytes(orphanBytes), "text-rose-600 dark:text-rose-400"),
		},
	}
}

func statCard(title, value, colorClass string) core.Node {
	return core.View{
		Class: "flex-1 flex-col p-3 bg-white dark:bg-zinc-800 rounded-lg shadow-sm border border-slate-200 dark:border-zinc-700",
		Children: []core.Node{
			core.Text{
				Class:   "text-xs text-slate-500 dark:text-zinc-400 font-medium",
				Content: title,
			},
			core.Text{
				Class:   "text-lg font-bold " + colorClass,
				Content: value,
			},
		},
	}
}

func renderToolbar(cx *core.Scope, state *AppState) core.Node {
	isScanning := state.IsScanning.Get()
	isMigrating := state.IsMigrating.Get()
	hasTarget := strings.TrimSpace(state.TargetDir.Get()) != ""
	selected := countSelected(state)

	scanLabel := "Rescan Caches"
	if isScanning {
		scanLabel = "Scanning..."
	}

	migrateLabel := fmt.Sprintf("Migrate %d Selected", selected)
	if isMigrating {
		migrateLabel = "Migrating..."
	}

	return core.View{
		Class: "flex-row justify-between items-center gap-3 py-1",
		Children: []core.Node{
			ui.Button{
				Text:     scanLabel,
				Variant:  ui.ButtonSecondary,
				Icon:     lucide.RefreshCw,
				Disabled: func() bool { return busy(state) },
				OnPress:  func() { triggerScan(state) },
				TestID:   "rescan",
			},
			core.View{
				Class: "flex-row items-center gap-2",
				Children: []core.Node{
					core.Show{
						When: func() bool { return state.IsMigrating.Get() },
						Then: func(cx *core.Scope) core.Node {
							return ui.Button{
								Text:     "Cancel",
								Variant:  ui.ButtonSecondary,
								Icon:     lucide.CircleX,
								Disabled: func() bool { return state.CancelRequested.Get() },
								OnPress:  func() { state.CancelRequested.Set(true) },
								TestID:   "cancel-move",
							}
						},
					},
					ui.Button{
						Text:     migrateLabel,
						Variant:  ui.ButtonPrimary,
						Icon:     lucide.ArrowUpRight,
						Disabled: func() bool { return busy(state) || !hasTarget || selected == 0 },
						OnPress:  func() { state.ConfirmOpen.Set(true) },
						TestID:   "migrate",
					},
				},
			},
		},
	}
}

func renderTargetRow(cx *core.Scope, state *AppState) core.Node {
	return core.View{
		Class: "flex-col gap-1",
		Children: []core.Node{
			core.View{
				Class: "flex-row items-center gap-2",
				Children: []core.Node{
					core.Text{
						Class:   "text-xs font-semibold text-slate-600 dark:text-zinc-300",
						Content: "Target folder",
					},
					ui.Input{Input: core.Input{
						Class:       "flex-1",
						Value:       func() string { return state.TargetDir.Get() },
						Placeholder: "Choose an empty folder on the external volume",
						OnChange: func(text string) {
							state.TargetDir.Set(strings.TrimSpace(text))
							triggerTargetCheck(state)
						},
						TestID: "target",
					}},
					ui.Button{
						Text:    "Browse...",
						Variant: ui.ButtonSecondary,
						Icon:    lucide.Folder,
						OnPress: func() { state.PickerOpen.Set(true) },
						TestID:  "browse",
					},
				},
			},
			core.Show{
				When: func() bool { return state.TargetInfo.Get() != "" },
				Then: func(cx *core.Scope) core.Node {
					return core.Text{
						ClassFunc:   func() string { return targetInfoClass(state.TargetInfo.Get()) },
						ContentFunc: func() string { return state.TargetInfo.Get() },
						TestID:      "target-info",
					}
				},
			},
		},
	}
}

// targetInfoClass colors the volume summary. A problem reads in the warning
// color, a ready folder in the calm one.
func targetInfoClass(info string) string {
	if strings.HasPrefix(info, "Problem:") {
		return "text-xs text-rose-600 dark:text-rose-400"
	}
	return "text-xs text-emerald-600 dark:text-emerald-400"
}

// renderGuide shows the three steps while no target folder is set, so the first
// run needs no manual.
func renderGuide(state *AppState) core.Node {
	return core.Show{
		When: func() bool { return strings.TrimSpace(state.TargetDir.Get()) == "" },
		Then: func(cx *core.Scope) core.Node {
			return core.View{
				Class:  "flex-col gap-1 p-3 bg-slate-100 dark:bg-zinc-800 rounded-lg",
				TestID: "guide",
				Children: []core.Node{
					core.Text{
						Class:   "text-xs font-semibold text-slate-700 dark:text-zinc-200",
						Content: "Three steps",
					},
					guideStep("1", "Choose an empty folder on the external volume."),
					guideStep("2", "Check the caches and the space the move frees."),
					guideStep("3", "Press Migrate, then confirm."),
				},
			}
		},
	}
}

func guideStep(number, text string) core.Node {
	return core.View{
		Class: "flex-row items-center gap-2",
		Children: []core.Node{
			core.Text{Class: "text-xs font-bold text-slate-500 dark:text-zinc-400", Content: number + "."},
			core.Text{Class: "text-xs text-slate-600 dark:text-zinc-300", Content: text},
		},
	}
}

func renderItemList(cx *core.Scope, state *AppState) core.Node {
	return core.Scroll{
		Class: "flex-1 flex-col gap-2 w-full",
		Children: []core.Node{
			core.Each(
				func() []exoCore.CacheItem { return state.Items.Get() },
				func(it exoCore.CacheItem) string { return it.ID },
				func(cx *core.Scope, item func() exoCore.CacheItem, _ func() int) core.Node {
					return renderItemRow(state, item)
				},
			),
			core.Dynamic(func(cx *core.Scope) core.Node {
				if len(state.Items.Get()) > 0 {
					return core.View{}
				}
				return core.Text{
					Class:   "text-sm text-slate-500 dark:text-zinc-400 text-center py-8",
					Content: "No caches detected. Press Rescan Caches.",
				}
			}),
		},
	}
}

func renderItemRow(state *AppState, item func() exoCore.CacheItem) core.Node {
	id := item().ID

	// The row reads the item through funcs. A migration changes the item in
	// place, so a value read at build time would keep the old badge.
	statusBadge := func() string {
		switch it := item(); {
		case it.IsMigrated:
			return "Migrated"
		case !it.IsActive:
			return "Abandoned"
		case it.BlockedReason != "":
			return "Blocked"
		default:
			return "Local SSD"
		}
	}
	badgeClass := func() string {
		switch it := item(); {
		case it.IsMigrated:
			return "text-xs px-2 py-0.5 rounded-full font-medium bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-300"
		case !it.IsActive, it.BlockedReason != "":
			return "text-xs px-2 py-0.5 rounded-full font-medium bg-rose-100 text-rose-800 dark:bg-rose-950 dark:text-rose-300"
		default:
			return "text-xs px-2 py-0.5 rounded-full font-medium bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-300"
		}
	}
	selectable := func() bool {
		it := item()
		return !it.IsMigrated && it.IsActive && it.BlockedReason == ""
	}

	return core.View{
		Class:  "flex-col p-3 bg-white dark:bg-zinc-800 rounded-md border border-slate-200 dark:border-zinc-700 gap-1",
		TestID: "row-" + id,
		Children: []core.Node{
			core.View{
				Class: "flex-row justify-between items-center",
				Children: []core.Node{
					core.View{
						Class: "flex-row items-center gap-2",
						Children: []core.Node{
							ui.Checkbox{Checked: func() bool {
								return state.Selected.Get()[id]
							}, Disabled: func() bool { return !selectable() }, TestID: "select-" + id, OnChange: func(bool) {
								toggleSelected(state, id)
							}},
							core.Text{
								Class:       "font-semibold text-sm text-slate-800 dark:text-zinc-100",
								ContentFunc: func() string { return item().Title },
							},
						},
					},
					core.View{
						Class: "flex-row items-center gap-2",
						Children: []core.Node{
							core.Text{
								Class:       "text-xs font-bold text-slate-700 dark:text-zinc-300",
								ContentFunc: func() string { return internalUi.FormatBytes(item().SizeBytes) },
							},
							core.Text{
								ClassFunc:   badgeClass,
								ContentFunc: statusBadge,
								TestID:      "badge-" + id,
							},
							core.Show{
								When: func() bool { return item().IsMigrated },
								Then: func(cx *core.Scope) core.Node {
									return ui.Button{
										Text:     "Roll back",
										Variant:  ui.ButtonSecondary,
										Icon:     lucide.Undo2,
										TestID:   "rollback-" + id,
										Disabled: func() bool { return busy(state) },
										OnPress:  func() { state.RollbackTarget.Set(id) },
									}
								},
							},
							core.Show{
								When: func() bool { return item().Category == exoCore.CategoryOrphan },
								Then: func(cx *core.Scope) core.Node {
									return ui.Button{
										Text:     "Delete",
										Variant:  ui.ButtonSecondary,
										Icon:     lucide.Trash,
										TestID:   "clean-" + id,
										Disabled: func() bool { return busy(state) },
										OnPress:  func() { state.CleanTarget.Set(id) },
									}
								},
							},
						},
					},
				},
			},
			core.Text{
				Class:       "text-xs font-mono text-slate-400 dark:text-zinc-500 truncate",
				ContentFunc: func() string { return item().CurrentPath },
			},
			renderBlockedText(item),
			renderWarningText(item().Warning),
		},
	}
}

// renderBlockedText explains why Exodrive cannot move an item. A blocked row
// stays visible, so the reason sits next to the row it belongs to.
func renderBlockedText(item func() exoCore.CacheItem) core.Node {
	return core.Show{
		When: func() bool { return item().BlockedReason != "" },
		Then: func(cx *core.Scope) core.Node {
			return core.Text{
				Class:       "text-xs text-rose-600 dark:text-rose-400",
				ContentFunc: func() string { return item().BlockedReason },
				TestID:      "blocked-" + item().ID,
			}
		},
	}
}

func renderWarningText(warning string) core.Node {
	if warning == "" {
		return core.View{}
	}
	return core.Text{
		Class:   "text-xs text-amber-600 dark:text-amber-400",
		Content: "Warning: " + warning,
	}
}

func renderFooter(cx *core.Scope, state *AppState) core.Node {
	return core.View{
		Class: "flex-row justify-between items-center pt-2 border-t border-slate-200 dark:border-zinc-800",
		Children: []core.Node{
			core.Text{
				Class:       "text-xs text-slate-400 dark:text-zinc-500",
				ContentFunc: func() string { return progressText(state) },
			},
			core.Text{
				Class:   "text-xs font-mono text-slate-500 dark:text-zinc-400",
				Content: fmt.Sprintf("Ledger: %s", exoCore.GetManifestPath()),
			},
		},
	}
}

func progressText(state *AppState) string {
	switch {
	case state.IsMigrating.Get():
		total := state.MoveTotal.Get()
		done := state.MoveDone.Get()
		current := state.MoveCurrent.Get()
		if total == 0 {
			return "Moving the caches..."
		}
		return fmt.Sprintf("Moving %d of %d: %s, %.0f%%", done+1, total, current, state.ProgressPct.Get()*100)
	case state.IsRollingBack.Get():
		return "Moving the cache back to the internal disk..."
	case state.IsCleaning.Get():
		return "Deleting the abandoned cache..."
	default:
		return "Ready."
	}
}

func renderPicker(state *AppState) core.Node {
	return filepicker.Dialog{
		State:  state.Picker,
		Open:   func() bool { return state.PickerOpen.Get() },
		OnOpen: func(open bool) { state.PickerOpen.Set(open) },
		Labels: filepicker.Labels{
			Title:   "Choose target folder",
			Address: "Folder",
			Icons:   "Icons",
			List:    "List",
			Name:    "Name",
			Close:   "Close",
			Empty:   "This folder is empty",
			Cancel:  "Cancel",
			Choose:  "Choose",
		},
		Device: state.PickerDevice,
		Hint:   "Exodrive copies each selected cache into this folder.",
		Choose: func(path string) { state.TargetDir.Set(path) },
		TestID: "target-picker",
	}
}

func renderConfirm(cx *core.Scope, state *AppState) core.Node {
	// The description reads the selection and the target folder, so it lives
	// in a Dynamic: a signal read at build time would not repaint.
	return core.Dynamic(func(cx *core.Scope) core.Node {
		return ui.AlertDialog{
			Open: func() bool { return state.ConfirmOpen.Get() },
			OnOpenChange: func(open bool, reason base.Reason) {
				if open || reason == base.ReasonSelect {
					return
				}
				state.ConfirmOpen.Set(false)
				state.StatusMsg.Set("Move cancelled. Nothing changed.")
			},
			Title: "Move selected caches?",
			Description: fmt.Sprintf(
				"Exodrive copies %d cache(s) to %s, checks every copy, then removes the local copy. Run 'exodrive rollback' to move a cache back.",
				countSelected(state), state.TargetDir.Get()),
			Actions: []ui.AlertAction{
				{Text: "Cancel", Variant: ui.ButtonSecondary, TestID: "confirm-cancel",
					OnPress: func() { state.ConfirmOpen.Set(false) }},
				{Text: "Move caches", Variant: ui.ButtonPrimary, TestID: "confirm-move",
					OnPress: func() { state.ConfirmOpen.Set(false); triggerMigration(state) }},
			},
			TestID: "confirm",
		}
	})
}

// busy reports whether a long action runs. A second action waits for it.
func busy(state *AppState) bool {
	return state.IsScanning.Get() || state.IsMigrating.Get() || state.IsRollingBack.Get() || state.IsCleaning.Get()
}

func countSelected(state *AppState) int {
	selected := state.Selected.Get()
	count := 0
	for _, it := range state.Items.Get() {
		if selected[it.ID] && !it.IsMigrated && it.IsActive && it.BlockedReason == "" {
			count++
		}
	}
	return count
}

// migrationDestination returns the external path that holds the migrated copy.
func migrationDestination(id string) string {
	m, err := exoCore.LoadManifest()
	if err != nil {
		return "the external volume"
	}
	if rec, ok := m.Records[id]; ok {
		return rec.DestinationDir
	}
	return "the external volume"
}

func renderRollbackConfirm(state *AppState) core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		id := state.RollbackTarget.Get()
		return ui.AlertDialog{
			Open: func() bool { return state.RollbackTarget.Get() != "" },
			OnOpenChange: func(open bool, reason base.Reason) {
				if open || reason == base.ReasonSelect {
					return
				}
				state.RollbackTarget.Set("")
				state.StatusMsg.Set("Rollback cancelled. Nothing changed.")
			},
			Title: "Move this cache back?",
			Description: fmt.Sprintf(
				"Exodrive copies the cache from %s back to its original place. It checks the copy, then removes the external copy. The ledger record is removed.",
				migrationDestination(id)),
			Actions: []ui.AlertAction{
				{Text: "Cancel", Variant: ui.ButtonSecondary, TestID: "rollback-cancel",
					OnPress: func() { state.RollbackTarget.Set("") }},
				{Text: "Move back", Variant: ui.ButtonPrimary, TestID: "rollback-confirm",
					OnPress: func() { state.RollbackTarget.Set(""); triggerRollback(state, id) }},
			},
			TestID: "rollback-dialog",
		}
	})
}

// itemByID returns the item with this ID, and false when the scan no longer
// holds it.
func itemByID(state *AppState, id string) (exoCore.CacheItem, bool) {
	for _, it := range state.Items.GetUntracked() {
		if it.ID == id {
			return it, true
		}
	}
	return exoCore.CacheItem{}, false
}

func renderCleanConfirm(state *AppState) core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		id := state.CleanTarget.Get()
		item, found := itemByID(state, id)
		description := "Exodrive deletes this folder from the internal disk. This action cannot be undone."
		if found {
			description = fmt.Sprintf(
				"Exodrive deletes %s (%s) from the internal disk. This action cannot be undone.",
				item.CurrentPath, internalUi.FormatBytes(item.SizeBytes))
		}
		return ui.AlertDialog{
			Open: func() bool { return state.CleanTarget.Get() != "" },
			OnOpenChange: func(open bool, reason base.Reason) {
				if open || reason == base.ReasonSelect {
					return
				}
				state.CleanTarget.Set("")
				state.StatusMsg.Set("Delete cancelled. Nothing changed.")
			},
			Title:       "Delete this abandoned cache?",
			Description: description,
			Actions: []ui.AlertAction{
				{Text: "Cancel", Variant: ui.ButtonSecondary, TestID: "clean-cancel",
					OnPress: func() { state.CleanTarget.Set("") }},
				{Text: "Delete", Variant: ui.ButtonPrimary, TestID: "clean-confirm",
					OnPress: func() { state.CleanTarget.Set(""); triggerClean(state, id) }},
			},
			TestID: "clean-dialog",
		}
	})
}

// triggerTargetCheck reports the volume under the target folder, or the reason
// it fails. It runs after every edit, and it drops a result that a newer edit
// made stale.
func triggerTargetCheck(state *AppState) {
	target := strings.TrimSpace(state.TargetDir.Get())
	if target == "" {
		state.TargetInfo.Set("")
		return
	}

	go func() {
		info, err := state.Engine.CheckTarget(target)
		state.Root.Post(func() {
			if strings.TrimSpace(state.TargetDir.Get()) != target {
				return
			}
			if err != nil {
				state.TargetInfo.Set("Problem: " + err.Error())
				return
			}
			state.TargetInfo.Set(fmt.Sprintf("Ready. %s, %s free.", strings.ToUpper(info.Filesystem), internalUi.FormatBytes(info.FreeBytes)))
		})
	}()
}

func toggleSelected(state *AppState, id string) {
	next := make(map[string]bool, len(state.Selected.GetUntracked()))
	for key, value := range state.Selected.GetUntracked() {
		next[key] = value
	}
	next[id] = !next[id]
	state.Selected.Set(next)
}

func triggerScan(state *AppState) {
	if busy(state) {
		return
	}
	state.IsScanning.Set(true)
	state.StatusMsg.Set("Scanning developer caches...")

	go func() {
		items, err := state.Engine.ScanAll(context.Background())
		state.Root.Post(func() {
			state.IsScanning.Set(false)
			if err != nil && len(items) == 0 {
				state.StatusMsg.Set(fmt.Sprintf("Scan failed: %v", err))
				return
			}
			state.Items.Set(items)

			// Select every active, unmigrated cache.
			selected := make(map[string]bool)
			for _, it := range items {
				if !it.IsMigrated && it.IsActive {
					selected[it.ID] = true
				}
			}
			state.Selected.Set(selected)

			if err != nil {
				state.StatusMsg.Set(fmt.Sprintf("Scan complete. Found %d cache(s). Some toolchains failed: %v", len(items), err))
				return
			}
			state.StatusMsg.Set(fmt.Sprintf("Scan complete. Found %d cache(s).", len(items)))
		})
	}()
}

func triggerMigration(state *AppState) {
	if busy(state) {
		return
	}

	targetDir := strings.TrimSpace(state.TargetDir.Get())
	if targetDir == "" {
		state.StatusMsg.Set("Choose a target folder first.")
		return
	}

	selected := state.Selected.Get()
	var toMigrate []exoCore.CacheItem
	for _, it := range state.Items.Get() {
		if selected[it.ID] && !it.IsMigrated && it.IsActive && it.BlockedReason == "" {
			toMigrate = append(toMigrate, it)
		}
	}
	if len(toMigrate) == 0 {
		state.StatusMsg.Set("No cache is selected for migration.")
		return
	}

	state.IsMigrating.Set(true)
	state.CancelRequested.Set(false)
	state.ProgressPct.Set(0)
	state.MoveTotal.Set(len(toMigrate))
	state.MoveDone.Set(0)
	state.MoveCurrent.Set(toMigrate[0].Title)
	state.StatusMsg.Set(fmt.Sprintf("Migrating %d cache(s)...", len(toMigrate)))

	go func() {
		ctx := context.Background()
		successCount := 0
		stopped := false
		var errs []string

		for index, it := range toMigrate {
			if state.CancelRequested.GetUntracked() {
				stopped = true
				break
			}
			state.Root.Post(func() {
				state.MoveCurrent.Set(it.Title)
				state.MoveDone.Set(index)
				state.ProgressPct.Set(0)
			})
			// Report 1% steps: a large file copies in 1 MiB chunks and would
			// otherwise post thousands of updates.
			lastPct := float32(-1)
			err := state.Engine.Migrate(ctx, it, targetDir, func(copied, total int64) {
				if total <= 0 {
					return
				}
				pct := float32(copied) / float32(total)
				if pct > 1 {
					pct = 1
				}
				if pct-lastPct < 0.01 && pct < 1 {
					return
				}
				lastPct = pct
				state.Root.Post(func() { state.ProgressPct.Set(pct) })
			})
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", it.Title, err))
			} else {
				successCount++
			}
		}

		items, scanErr := state.Engine.ScanAll(ctx)
		if scanErr != nil {
			errs = append(errs, fmt.Sprintf("rescan: %v", scanErr))
		}

		state.Root.Post(func() {
			if scanErr == nil {
				state.Items.Set(items)
				state.Selected.Set(map[string]bool{})
			}
			state.IsMigrating.Set(false)
			state.CancelRequested.Set(false)
			state.ProgressPct.Set(0)
			state.MoveTotal.Set(0)
			state.MoveDone.Set(0)
			state.MoveCurrent.Set("")
			if stopped {
				state.StatusMsg.Set(fmt.Sprintf("Stopped after %d of %d moves. Press Rescan Caches to see the list.", successCount, len(toMigrate)))
				return
			}
			if len(errs) > 0 {
				state.StatusMsg.Set(fmt.Sprintf("Migrated %d cache(s). %d failed: %s", successCount, len(errs), strings.Join(errs, "; ")))
			} else {
				state.StatusMsg.Set(fmt.Sprintf("Migrated %d cache(s) to %s.", successCount, targetDir))
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
		state.StatusMsg.Set("This cache is no longer in the list. Scan again.")
		return
	}

	state.IsCleaning.Set(true)
	state.StatusMsg.Set(fmt.Sprintf("Deleting %s...", item.Title))

	go func() {
		ctx := context.Background()
		err := state.Engine.CleanOrphan(item.CurrentPath)
		items, scanErr := state.Engine.ScanAll(ctx)

		state.Root.Post(func() {
			state.IsCleaning.Set(false)
			if scanErr == nil {
				state.Items.Set(items)
				state.Selected.Set(map[string]bool{})
			}
			if err != nil {
				state.StatusMsg.Set(fmt.Sprintf("Delete failed: %v", err))
				return
			}
			state.StatusMsg.Set(fmt.Sprintf("Deleted %s. %s is free.", item.Title, internalUi.FormatBytes(item.SizeBytes)))
		})
	}()
}

func triggerRollback(state *AppState, id string) {
	if busy(state) {
		return
	}
	state.IsRollingBack.Set(true)
	state.StatusMsg.Set("Moving the cache back...")

	go func() {
		ctx := context.Background()
		err := state.Engine.Rollback(ctx, id)
		items, scanErr := state.Engine.ScanAll(ctx)

		state.Root.Post(func() {
			state.IsRollingBack.Set(false)
			if scanErr == nil {
				state.Items.Set(items)
				state.Selected.Set(map[string]bool{})
			}
			if err != nil {
				state.StatusMsg.Set(fmt.Sprintf("Rollback failed: %v", err))
				return
			}
			state.StatusMsg.Set("Cache moved back. The ledger record is removed.")
		})
	}()
}
