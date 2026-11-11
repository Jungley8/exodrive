package desktop

import (
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/icons/lucide"
	"github.com/oneo-me/nsgo/ui"

	exoCore "exodrive/internal/core"
)

// renderRow draws one cache as a single line: what it is, how big it is, and
// where it lives. Paths, the switch method and the row's actions wait under the
// chevron, so the list reads before it explains.
func renderRow(cx *core.Scope, state *AppState, item func() exoCore.CacheItem) core.Node {
	colors := ui.ColorsOf(cx)
	it := item()
	id := it.ID
	place := stateOf(it)
	open := state.OpenRows.Get()[id]
	failure, failed := state.Failures.Get()[id]
	selectable := movable(it)

	children := []core.Node{rowHead(cx, state, it, place, failed, open, selectable)}
	if line := rowProblem(cx, it, place, failed); line != nil {
		children = append(children, line)
	}
	if open {
		children = append(children, rowDetails(cx, state, it, place, failure))
	}

	return core.View{
		Style: core.Style{Direction: core.Column, Gap: 2, Padding: core.Symmetric(9, 12), CornerRadius: 10,
			BorderCurve: core.BorderCurveContinuous, BorderWidth: 1},
		StyleFunc: func() core.Style {
			s := core.Style{Background: colors.Surface(), BorderColor: colors.Border()}
			if failed {
				s.BorderColor = colors.Danger()
			}
			return s
		},
		TestID:   "row-" + id,
		Children: children,
	}
}

// rowHead is the always-visible line of a cache.
func rowHead(cx *core.Scope, state *AppState, it exoCore.CacheItem, place cacheState, failed, open, selectable bool) core.Node {
	colors := ui.ColorsOf(cx)
	id := it.ID
	label := it.Title
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10}, Children: []core.Node{
		ui.Checkbox{
			Checked:  func() bool { return state.Selected.Get()[id] },
			Disabled: func() bool { return !selectable },
			OnChange: func(bool) { toggleSelected(state, id) },
			Label:    "Select " + label,
			TestID:   "select-" + id,
		},
		core.Text{
			Content:   label,
			MaxLines:  1,
			Style:     core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), FontSize: 13, LineHeight: 18, FontWeight: ui.Medium},
			StyleFunc: func() core.Style { return core.Style{Color: colors.Text()} },
		},
		typeLine(bytes(it.SizeBytes), figures(typeSmall), colors.Muted),
		stateChip(cx, place, failed, id),
		ui.IconButton{
			IconFunc: func() *core.SVG {
				if open {
					return lucide.ChevronDown()
				}
				return lucide.ChevronRight()
			},
			Label:    "Details for " + label,
			Size:     24,
			IconSize: 14,
			Tint:     colors.Muted,
			OnPress:  func() { toggleOpen(state, id) },
			TestID:   "expand-" + id,
		},
	}}
}

// stateChip says where the cache lives. It carries the word, so the color is
// never the only signal.
func stateChip(cx *core.Scope, place cacheState, failed bool, id string) core.Node {
	colors := ui.ColorsOf(cx)
	tints := ui.TintsOf(cx)
	label := place.label()
	fill, ink := tints.Warn, colors.Warn
	switch {
	case failed && place == stateMoved:
		label, fill, ink = "Move back failed", tints.Danger, colors.Danger
	case failed:
		label, fill, ink = "Move failed", tints.Danger, colors.Danger
	case place == stateMoved:
		fill, ink = tints.Up, colors.Up
	case place == stateBlocked, place == stateUnused:
		fill, ink = tints.Danger, colors.Danger
	}
	return core.View{
		Style:     core.Style{Padding: core.Symmetric(2, 8), CornerRadius: 999},
		StyleFunc: ui.Filled(fill),
		Children: []core.Node{
			core.Text{Content: label, Style: core.Style{FontSize: 11.5, LineHeight: 15, FontWeight: ui.Medium},
				StyleFunc: func() core.Style { return core.Style{Color: ink()} }, TestID: "badge-" + id},
		},
	}
}

// rowProblem is the line a cache needs even while its details stay closed: why
// it cannot move, a warning, or that a move stopped.
func rowProblem(cx *core.Scope, it exoCore.CacheItem, place cacheState, failed bool) core.Node {
	colors := ui.ColorsOf(cx)
	switch {
	case it.BlockedReason != "":
		return inlineNote(cx, lucide.TriangleAlert, colors.Danger, it.BlockedReason)
	case failed:
		return inlineNote(cx, lucide.TriangleAlert, colors.Danger, "Open the cache to read the reason and try again.")
	case it.Warning != "":
		return inlineNote(cx, lucide.TriangleAlert, colors.Warn, it.Warning)
	case place == stateUnused:
		return inlineNote(cx, lucide.Info, colors.Muted, "The program does not use this cache anymore.")
	}
	return nil
}

func inlineNote(cx *core.Scope, icon func() *core.SVG, c func() core.Color, content string) core.Node {
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignStart, Gap: 6, Padding: core.Edges{Left: 26, Top: 2}},
		Children: []core.Node{
			ui.Icon(icon, 12, c),
			core.Text{Content: content, Style: core.Style{FlexShrink: 1, FontSize: 12, LineHeight: 16},
				StyleFunc: func() core.Style { return core.Style{Color: c()} }},
		}}
}

// rowDetails is what a cache shows once opened: where it is, what Exodrive
// changes, why a move stopped, and the action that fits its state.
func rowDetails(cx *core.Scope, state *AppState, it exoCore.CacheItem, place cacheState, failure friendly) core.Node {
	colors := ui.ColorsOf(cx)
	id := it.ID

	rows := []core.Node{
		core.View{Style: core.Style{Height: core.Pt(1), Margin: core.Edges{Top: 8, Bottom: 8}}, StyleFunc: ui.Filled(colors.Line)},
		pathLine(cx, "Folder", it.CurrentPath, "path-"+id),
	}
	if it.DestinationPath != "" {
		rows = append(rows, pathLine(cx, "Copy", it.DestinationPath, "destination-"+id))
	}
	if note := switchNote(it.RedirectMethod); note != "" {
		rows = append(rows, typeLine(note, typeSmall, colors.Muted))
	}
	if failure.Title != "" {
		rows = append(rows, failureBlock(cx, state, it, failure))
	}

	action := rowAction(state, it, place)
	if action != nil {
		rows = append(rows, core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Padding: core.Edges{Top: 4}}, Children: []core.Node{action}})
	}
	return core.View{Style: core.Style{Direction: core.Column, Gap: 4, Padding: core.Edges{Left: 26}}, Children: rows}
}

// pathLine names a path under the row. A path is read character by character,
// so it keeps the fixed-pitch face.
func pathLine(cx *core.Scope, name, path, testID string) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6}, Children: []core.Node{
		typeLine(name, typeSmall, colors.Muted),
		core.Text{Content: shortPath(path), MaxLines: 1, TestID: testID,
			Style:     core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), FontSize: 11.5, LineHeight: 16, FontFamily: ui.Mono},
			StyleFunc: func() core.Style { return core.Style{Color: colors.Muted()} }},
	}}
}

// failureBlock puts the reason a move stopped where the cache is, with the next
// step under it.
func failureBlock(cx *core.Scope, state *AppState, it exoCore.CacheItem, failure friendly) core.Node {
	colors := ui.ColorsOf(cx)
	tints := ui.TintsOf(cx)
	lines := []core.Node{typeLine(failure.Title, typeLabel, colors.Danger), typeLine(failure.Body, typeSmall, colors.Text)}
	if failure.Hint != "" {
		lines = append(lines, typeLine(failure.Hint, typeSmall, colors.Muted))
	}
	lines = append(lines, core.View{Style: core.Style{Direction: core.Row, Padding: core.Edges{Top: 4}}, Children: []core.Node{
		ui.Button{Text: "Try again", Variant: ui.ButtonSecondary, Size: ui.SizeSmall, Icon: lucide.RotateCcw,
			Disabled: func() bool { return busy(state) }, OnPress: func() { retry(state, it) }, TestID: "retry-" + it.ID},
	}})
	return core.View{
		Style:     core.Style{Direction: core.Column, Gap: 2, Padding: core.Symmetric(8, 10), CornerRadius: 8, Margin: core.Edges{Top: 4, Bottom: 4}},
		StyleFunc: ui.Filled(tints.Danger),
		Children:  lines,
	}
}

// retry starts the action that stopped again. Each one asks before it runs, so
// a retry never moves data on one press alone.
func retry(state *AppState, it exoCore.CacheItem) {
	switch {
	case it.IsMigrated:
		state.RollbackTarget.Set(it.ID)
	case !it.IsActive:
		state.CleanTarget.Set(it.ID)
	default:
		state.Selected.Set(map[string]bool{it.ID: true})
		state.ConfirmOpen.Set(true)
	}
}

// rowAction is the one action a cache offers by its state: move a cache back,
// or delete an unused one. A cache that can move already has the checkbox.
func rowAction(state *AppState, it exoCore.CacheItem, place cacheState) core.Node {
	switch place {
	case stateMoved:
		return ui.Button{Text: "Move back", Variant: ui.ButtonSecondary, Size: ui.SizeSmall, Icon: lucide.Undo2,
			Disabled: func() bool { return busy(state) },
			OnPress:  func() { state.RollbackTarget.Set(it.ID) },
			TestID:   "rollback-" + it.ID}
	case stateUnused:
		return ui.Button{Text: "Delete cache", Variant: ui.ButtonSecondary, Size: ui.SizeSmall, Icon: lucide.Trash,
			Disabled: func() bool { return busy(state) },
			OnPress:  func() { state.CleanTarget.Set(it.ID) },
			TestID:   "clean-" + it.ID}
	}
	return nil
}
