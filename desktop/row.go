package desktop

import (
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/icons/lucide"
	"github.com/oneo-me/nsgo/ui"

	exoCore "exodrive/internal/core"
)

// renderRow draws one cache as a card. Everything the cache has to say stays
// on the card: its size, its state, its folder, its copy, the change Exodrive
// makes, and the one action that fits the state. No fact waits behind a press.
func renderRow(cx *core.Scope, state *AppState, item func() exoCore.CacheItem) core.Node {
	colors := ui.ColorsOf(cx)
	it := item()
	id := it.ID
	place := stateOf(it)
	failure, failed := state.Failures.Get()[id]

	children := []core.Node{rowHead(cx, state, it, place, failed)}
	if it.CurrentPath != "" {
		children = append(children, pathLine(cx, state.Text.Folder, it.CurrentPath, "path-"+id))
	}
	if it.DestinationPath != "" {
		children = append(children, pathLine(cx, state.Text.Copy, it.DestinationPath, "destination-"+id))
		if line := checkCardLine(cx, state, it); line != nil {
			children = append(children, line)
		}
	}
	if line := rowProblem(cx, it, place, failed, state.Text); line != nil {
		children = append(children, line)
	}
	if failure.Title != "" {
		children = append(children, failureBlock(cx, state, it, failure))
	}
	if foot := rowFoot(cx, state, it, place); foot != nil {
		children = append(children, foot)
	}

	return core.View{
		Style: core.Style{Direction: core.Column, Gap: 6, Padding: core.Symmetric(12, 14),
			CornerRadius: 12, BorderCurve: core.BorderCurveContinuous, BorderWidth: 1},
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

// rowHead is the first line of a card: what the cache is, how big it is, and
// where it lives.
func rowHead(cx *core.Scope, state *AppState, it exoCore.CacheItem, place cacheState, failed bool) core.Node {
	colors := ui.ColorsOf(cx)
	id := it.ID
	label := it.Title
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10}, Children: []core.Node{
		ui.Checkbox{
			Checked:  func() bool { return state.Selected.Get()[id] },
			Disabled: func() bool { return !movable(it) },
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
		stateChip(cx, place, failed, id, state.Text),
	}}
}

// stateChip says where the cache lives. It carries the word, so the color is
// never the only signal.
func stateChip(cx *core.Scope, place cacheState, failed bool, id string, t text) core.Node {
	colors := ui.ColorsOf(cx)
	tints := ui.TintsOf(cx)
	label := place.label(t)
	fill, ink := tints.Warn, colors.Warn
	switch {
	case failed && place == stateMoved:
		label, fill, ink = t.MoveBackFailed, tints.Danger, colors.Danger
	case failed:
		label, fill, ink = t.MoveFailed, tints.Danger, colors.Danger
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

// rowProblem is the line a cache needs beside its facts: why it cannot move, a
// warning, or that a move stopped.
func rowProblem(cx *core.Scope, it exoCore.CacheItem, place cacheState, failed bool, t text) core.Node {
	colors := ui.ColorsOf(cx)
	switch {
	case it.BlockedReason != "":
		return inlineNote(cx, lucide.TriangleAlert, colors.Danger, it.BlockedReason)
	case failed:
		return inlineNote(cx, lucide.TriangleAlert, colors.Danger, t.MoveStopped)
	case place == stateRewritten:
		return inlineNote(cx, lucide.TriangleAlert, colors.Warn, t.RewrittenNote)
	case it.Warning != "":
		return inlineNote(cx, lucide.TriangleAlert, colors.Warn, it.Warning)
	case place == stateUnused:
		return inlineNote(cx, lucide.Info, colors.Muted, t.UnusedNote)
	}
	return nil
}

func inlineNote(cx *core.Scope, icon func() *core.SVG, c func() core.Color, content string) core.Node {
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignStart, Gap: 6},
		Children: []core.Node{
			ui.Icon(icon, 12, c),
			core.Text{Content: content, Style: core.Style{FlexShrink: 1, FontSize: 12, LineHeight: 16},
				StyleFunc: func() core.Style { return core.Style{Color: c()} }},
		}}
}

// pathLine names a path under the card. A path is read character by character,
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
// step under it. A person can copy the details for a report without opening
// the footer dialog.
func failureBlock(cx *core.Scope, state *AppState, it exoCore.CacheItem, failure shownFailure) core.Node {
	colors := ui.ColorsOf(cx)
	tints := ui.TintsOf(cx)
	lines := []core.Node{typeLine(failure.Title, typeLabel, colors.Danger), typeLine(failure.Body, typeSmall, colors.Text)}
	if failure.Hint != "" {
		lines = append(lines, typeLine(failure.Hint, typeSmall, colors.Muted))
	}
	actions := []core.Node{
		ui.Button{Text: state.Text.TryAgain, Variant: ui.ButtonSecondary, Size: ui.SizeSmall, Icon: lucide.RotateCcw,
			Disabled: func() bool { return busy(state) }, OnPress: func() { retry(state, it) }, TestID: "retry-" + it.ID},
	}
	// A cache with a record can keep its local folder. This is the way out when
	// a tool wrote the folder again after a move. The card already offers that
	// action when the scan reads the folder as written again.
	if it.IsMigrated && it.LinkState != exoCore.LinkRewritten {
		actions = append(actions, ui.Button{Text: state.Text.KeepLocalAction, Variant: ui.ButtonSecondary, Size: ui.SizeSmall, Icon: lucide.FolderInput,
			Disabled: func() bool { return busy(state) }, OnPress: func() { state.KeepLocalTarget.Set(it.ID) }, TestID: "keep-local-" + it.ID})
	}
	actions = append(actions, ui.Button{Text: state.Text.ReportCopy, Variant: ui.ButtonGhost, Size: ui.SizeSmall, Icon: lucide.ClipboardCopy,
		OnPress: func() { copyFailure(cx, state, failure) }, TestID: "copy-" + it.ID})
	lines = append(lines, core.View{Style: core.Style{Direction: core.Row, Gap: 8, Padding: core.Edges{Top: 4}}, Children: actions})
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
	// A rollback on a folder that a tool wrote again fails again. Keep the
	// folder that is here instead.
	case it.LinkState == exoCore.LinkRewritten:
		state.KeepLocalTarget.Set(it.ID)
	case it.IsMigrated:
		state.RollbackTarget.Set(it.ID)
	case !it.IsActive:
		state.CleanTarget.Set(it.ID)
	default:
		moveOne(state, it)
	}
}

// moveOne asks for a confirmation that holds this cache alone. Without a
// checked folder the folder picker opens first, because a folder is the step
// that a move needs.
func moveOne(state *AppState, it exoCore.CacheItem) {
	if busy(state) {
		return
	}
	if !state.Target.Get().Ready {
		openPicker(state)
		return
	}
	state.Selected.Set(map[string]bool{it.ID: true})
	state.ConfirmOpen.Set(true)
}

// rowFoot names the change Exodrive makes, beside the actions a cache offers
// by its state.
func rowFoot(cx *core.Scope, state *AppState, it exoCore.CacheItem, place cacheState) core.Node {
	colors := ui.ColorsOf(cx)
	actions := rowActions(state, it, place)
	// A blocked cache never moves, so the card names no change. A cache that a
	// tool wrote again names the problem above instead.
	note := ""
	if place != stateBlocked && place != stateRewritten {
		note = switchNote(it.RedirectMethod, state.Text)
	}
	if len(actions) == 0 && note == "" {
		return nil
	}
	children := []core.Node{}
	if note != "" {
		children = append(children, core.Text{Content: note, MaxLines: 2,
			Style:     core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), FontSize: 12, LineHeight: 16},
			StyleFunc: func() core.Style { return core.Style{Color: colors.Muted()} }})
	} else {
		children = append(children, fill())
	}
	children = append(children, actions...)
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10}, Children: children}
}

// rowActions lists the actions a cache offers by its state: move it, check it,
// move it back, or delete it. A blocked cache offers none, because the reason
// on the card says what to fix first.
func rowActions(state *AppState, it exoCore.CacheItem, place cacheState) []core.Node {
	disabled := func() bool { return busy(state) }
	t := state.Text
	switch place {
	case stateLocal:
		return []core.Node{ui.Button{Text: t.Move, Variant: ui.ButtonSecondary, Size: ui.SizeSmall, Icon: lucide.ArrowRight,
			Disabled: disabled, OnPress: func() { moveOne(state, it) }, TestID: "move-" + it.ID}}
	case stateMoved:
		actions := make([]core.Node, 0, 2)
		if it.DestinationPath != "" {
			actions = append(actions, ui.Button{Text: t.CheckCopy, Variant: ui.ButtonGhost, Size: ui.SizeSmall, Icon: lucide.BadgeCheck,
				Disabled: disabled, OnPress: func() { triggerCheck(state, it) }, TestID: "check-" + it.ID})
		}
		return append(actions, ui.Button{Text: t.MoveBack, Variant: ui.ButtonSecondary, Size: ui.SizeSmall, Icon: lucide.Undo2,
			Disabled: disabled, OnPress: func() { state.RollbackTarget.Set(it.ID) }, TestID: "rollback-" + it.ID})
	case stateUnused:
		return []core.Node{ui.Button{Text: t.DeleteCache, Variant: ui.ButtonSecondary, Size: ui.SizeSmall, Icon: lucide.Trash,
			Disabled: disabled, OnPress: func() { state.CleanTarget.Set(it.ID) }, TestID: "clean-" + it.ID}}
	// A tool wrote the folder again, so the copy cannot come back. The way out
	// is to keep the folder that is here.
	case stateRewritten:
		return []core.Node{ui.Button{Text: t.KeepLocalAction, Variant: ui.ButtonSecondary, Size: ui.SizeSmall, Icon: lucide.FolderInput,
			Disabled: disabled, OnPress: func() { state.KeepLocalTarget.Set(it.ID) }, TestID: "keep-local-" + it.ID}}
	}
	return nil
}
