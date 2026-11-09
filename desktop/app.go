package desktop

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/icons/lucide"
	"github.com/oneo-me/nsgo/ui"

	exoCore "exodrive/internal/core"
	"exodrive/internal/plugins"
	internalUi "exodrive/internal/ui"
)

type AppState struct {
	Items       *core.Signal[[]exoCore.CacheItem]
	Selected    *core.Signal[map[string]bool]
	TargetDir   *core.Signal[string]
	StatusMsg   *core.Signal[string]
	IsScanning  *core.Signal[bool]
	IsMigrating *core.Signal[bool]
	ProgressPct *core.Signal[float32]
	Engine      *exoCore.Engine
}

func NewAppState(cx *core.Scope) *AppState {
	reg := plugins.NewRegistry()
	var accessors []exoCore.ProviderAccessor
	for _, p := range reg.All() {
		accessors = append(accessors, p)
	}

	defaultTarget := "/Volumes/ExternalDrive/exodrive-caches"

	return &AppState{
		Items:       core.NewSignalFunc(cx, []exoCore.CacheItem{}, nil),
		Selected:    core.NewSignalFunc(cx, make(map[string]bool), nil),
		TargetDir:   core.NewSignal(cx, defaultTarget),
		StatusMsg:   core.NewSignal(cx, "Ready to scan caches."),
		IsScanning:  core.NewSignal(cx, false),
		IsMigrating: core.NewSignal(cx, false),
		ProgressPct: core.NewSignal[float32](cx, 0.0),
		Engine:      exoCore.NewEngine(accessors...),
	}
}

func Root(cx *core.Scope) core.Node {
	state := NewAppState(cx)

	// Trigger initial scan on load
	triggerScan(cx, state)

	return core.View{
		Class: "flex-col w-full h-full bg-slate-50 dark:bg-zinc-900 p-5 gap-4",
		Children: []core.Node{
			renderHeader(cx, state),
			renderStats(cx, state),
			renderToolbar(cx, state),
			renderItemList(cx, state),
			renderFooter(cx, state),
		},
	}
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
						Content: "Detect and migrate developer tool caches to external storage",
					},
				},
			},
			core.View{
				Class: "flex-row gap-2 items-center",
				Children: []core.Node{
					core.Text{
						Class:   "text-xs font-semibold text-slate-600 dark:text-zinc-300",
						Content: "Target Volume:",
					},
					core.Text{
						Class:   "text-xs font-mono bg-white dark:bg-zinc-800 px-2 py-1 rounded border border-slate-300 dark:border-zinc-700 text-slate-700 dark:text-zinc-200",
						Content: state.TargetDir.Get(),
					},
				},
			},
		},
	}
}

func renderStats(cx *core.Scope, state *AppState) core.Node {
	items := state.Items.Get()
	var localBytes, migratedBytes, orphanBytes int64

	for _, it := range items {
		if it.Category == exoCore.CategoryOrphan {
			orphanBytes += it.SizeBytes
		} else if it.IsMigrated {
			migratedBytes += it.SizeBytes
		} else {
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

	scanLabel := "Rescan Caches"
	if isScanning {
		scanLabel = "Scanning..."
	}

	migrateLabel := "Migrate Selected"
	if isMigrating {
		migrateLabel = "Migrating..."
	}

	return core.View{
		Class: "flex-row justify-between items-center gap-3 py-1",
		Children: []core.Node{
			core.View{
				Class: "flex-row gap-2",
				Children: []core.Node{
					ui.Button{
						Text:     scanLabel,
						Variant:  ui.ButtonSecondary,
						Icon:     lucide.RefreshCw,
						Disabled: func() bool { return isScanning || isMigrating },
						OnPress: func() {
							triggerScan(cx, state)
						},
					},
					ui.Button{
						Text:     migrateLabel,
						Variant:  ui.ButtonPrimary,
						Icon:     lucide.ArrowUpRight,
						Disabled: func() bool { return isScanning || isMigrating },
						OnPress: func() {
							triggerMigration(cx, state)
						},
					},
				},
			},
			core.Text{
				Class:   "text-xs text-slate-500 dark:text-zinc-400 italic",
				Content: state.StatusMsg.Get(),
			},
		},
	}
}

func renderItemList(cx *core.Scope, state *AppState) core.Node {
	items := state.Items.Get()
	selectedMap := state.Selected.Get()

	var rows []core.Node
	for _, it := range items {
		itemCopy := it
		isSelected := selectedMap[it.ID]

		statusBadge := "Local SSD"
		badgeClass := "bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-300"
		if it.IsMigrated {
			statusBadge = "Migrated"
			badgeClass = "bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-300"
		} else if !it.IsActive {
			statusBadge = "Abandoned"
			badgeClass = "bg-rose-100 text-rose-800 dark:bg-rose-950 dark:text-rose-300"
		}

		checkLabel := "[ ]"
		if isSelected {
			checkLabel = "[✓]"
		}

		rows = append(rows, core.View{
			Class: "flex-col p-3 bg-white dark:bg-zinc-800 rounded-md border border-slate-200 dark:border-zinc-700 gap-1",
			Children: []core.Node{
				core.View{
					Class: "flex-row justify-between items-center",
					Children: []core.Node{
						core.View{
							Class: "flex-row items-center gap-2",
							Children: []core.Node{
								core.Text{
									Class:   "font-mono font-bold text-sm cursor-pointer text-indigo-600 dark:text-indigo-400",
									Content: checkLabel,
									OnPress: func(core.PressEvent) {
										cur := state.Selected.Get()
										cur[itemCopy.ID] = !cur[itemCopy.ID]
										state.Selected.Set(cur)
									},
								},
								core.Text{
									Class:   "font-semibold text-sm text-slate-800 dark:text-zinc-100",
									Content: it.Title,
								},
							},
						},
						core.View{
							Class: "flex-row items-center gap-2",
							Children: []core.Node{
								core.Text{
									Class:   "text-xs font-bold text-slate-700 dark:text-zinc-300",
									Content: internalUi.FormatBytes(it.SizeBytes),
								},
								core.Text{
									Class:   "text-xs px-2 py-0.5 rounded font-medium " + badgeClass,
									Content: statusBadge,
								},
							},
						},
					},
				},
				core.Text{
					Class:   "text-xs font-mono text-slate-400 dark:text-zinc-500 truncate",
					Content: it.CurrentPath,
				},
				renderWarningText(it.Warning),
			},
		})
	}

	return core.Scroll{
		Class:    "flex-1 flex-col gap-2 w-full",
		Children: rows,
	}
}

func renderWarningText(warning string) core.Node {
	if warning == "" {
		return core.View{}
	}
	return core.Text{
		Class:   "text-xs text-amber-600 dark:text-amber-400",
		Content: "⚠️ " + warning,
	}
}

func renderFooter(cx *core.Scope, state *AppState) core.Node {
	return core.View{
		Class: "flex-row justify-between items-center pt-2 border-t border-slate-200 dark:border-zinc-800",
		Children: []core.Node{
			core.Text{
				Class:   "text-xs text-slate-400 dark:text-zinc-500",
				Content: "Exodrive v1.0 • APFS / ext4",
			},
			core.Text{
				Class:   "text-xs font-mono text-slate-500 dark:text-zinc-400",
				Content: fmt.Sprintf("Manifest: %s", exoCore.GetManifestPath()),
			},
		},
	}
}

func triggerScan(cx *core.Scope, state *AppState) {
	state.IsScanning.Set(true)
	state.StatusMsg.Set("Scanning developer caches...")

	go func() {
		ctx := context.Background()
		items, err := state.Engine.ScanAll(ctx)
		cx.Post(func() {
			state.IsScanning.Set(false)
			if err != nil {
				state.StatusMsg.Set(fmt.Sprintf("Scan failed: %v", err))
				return
			}
			state.Items.Set(items)

			// Default selection: select all unmigrated active items
			sel := make(map[string]bool)
			for _, it := range items {
				if !it.IsMigrated && it.IsActive {
					sel[it.ID] = true
				}
			}
			state.Selected.Set(sel)
			state.StatusMsg.Set(fmt.Sprintf("Scan completed. Found %d caches.", len(items)))
		})
	}()
}

func triggerMigration(cx *core.Scope, state *AppState) {
	selectedMap := state.Selected.Get()
	items := state.Items.Get()
	targetDir := state.TargetDir.Get()

	var toMigrate []exoCore.CacheItem
	for _, it := range items {
		if selectedMap[it.ID] && !it.IsMigrated {
			toMigrate = append(toMigrate, it)
		}
	}

	if len(toMigrate) == 0 {
		state.StatusMsg.Set("No unmigrated items selected.")
		return
	}

	state.IsMigrating.Set(true)
	state.StatusMsg.Set(fmt.Sprintf("Migrating %d item(s)...", len(toMigrate)))

	go func() {
		ctx := context.Background()
		successCount := 0
		var errs []string

		for _, it := range toMigrate {
			err := state.Engine.Migrate(ctx, it, targetDir, nil)
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", it.Title, err))
			} else {
				successCount++
			}
		}

		// Rescan after migration
		updatedItems, _ := state.Engine.ScanAll(ctx)

		cx.Post(func() {
			state.IsMigrating.Set(false)
			state.Items.Set(updatedItems)
			if len(errs) > 0 {
				state.StatusMsg.Set(fmt.Sprintf("Migrated %d with errors: %s", successCount, strings.Join(errs, "; ")))
			} else {
				state.StatusMsg.Set(fmt.Sprintf("Migrated %d cache(s) to %s.", successCount, filepath.Base(targetDir)))
			}
		})
	}()
}
