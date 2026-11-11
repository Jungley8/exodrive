package desktop

import (
	"fmt"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/ui"
)

// renderDialogs stacks the three confirmations. Each one asks before data
// moves or leaves, and each one says what happens after.
func renderDialogs(cx *core.Scope, state *AppState) core.Node {
	return core.Group{
		confirmMove(cx, state),
		confirmMoveBack(cx, state),
		confirmDelete(cx, state),
	}
}

// confirmMove lists the caches that move, so a press never moves more than the
// person saw.
func confirmMove(cx *core.Scope, state *AppState) core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		colors := ui.ColorsOf(cx)
		selected := selectedItems(state)
		var total int64
		for _, it := range selected {
			total += it.SizeBytes
		}
		target := state.Target.Get()

		rows := make([]core.Node, 0, len(selected)+1)
		for i, it := range selected {
			if i == 5 {
				rows = append(rows, typeLine(fmt.Sprintf("and %d more", len(selected)-i), typeSmall, colors.Muted))
				break
			}
			rows = append(rows, core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
				core.Text{Content: it.Title, MaxLines: 1,
					Style:     core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), FontSize: 13, LineHeight: 18},
					StyleFunc: func() core.Style { return core.Style{Color: colors.Text()} }},
				typeLine(bytes(it.SizeBytes), figures(typeSmall), colors.Muted),
			}})
		}

		return ui.Dialog{
			Open: func() bool { return state.ConfirmOpen.Get() },
			OnOpenChange: func(open bool, _ base.Reason) {
				if open {
					return
				}
				state.ConfirmOpen.Set(false)
			},
			Title: fmt.Sprintf("Move %s?", count(len(selected), "cache", "caches")),
			Description: fmt.Sprintf("%s moves to %s. Exodrive checks each copy, then frees the space on this Mac.",
				bytes(total), shortPath(target.Path)),
			Children: []core.Node{core.View{Style: core.Style{Direction: core.Column, Gap: 4, Padding: core.Symmetric(8, 10), CornerRadius: 8},
				StyleFunc: ui.Filled(colors.Inset), Children: rows}},
			Footer: []core.Node{
				ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, TestID: "confirm-cancel",
					OnPress: func() { state.ConfirmOpen.Set(false); state.Status.Set("Nothing moved.") }},
				ui.Button{Text: fmt.Sprintf("Move %s", count(len(selected), "cache", "caches")), Variant: ui.ButtonPrimary, TestID: "confirm-move",
					OnPress: func() { state.ConfirmOpen.Set(false); triggerMigration(state) }},
			},
			TestID: "confirm",
		}
	})
}

// confirmMoveBack asks before a cache returns to this Mac.
func confirmMoveBack(cx *core.Scope, state *AppState) core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		id := state.RollbackTarget.Get()
		it, _ := itemByID(state, id)
		from := "the drive"
		if it.DestinationPath != "" {
			from = shortPath(it.DestinationPath)
		}
		return ui.AlertDialog{
			Open: func() bool { return state.RollbackTarget.Get() != "" },
			OnOpenChange: func(open bool, reason base.Reason) {
				if open {
					return
				}
				state.RollbackTarget.Set("")
				if reason != base.ReasonSelect {
					state.Status.Set("Nothing moved.")
				}
			},
			Title: "Move this cache back?",
			Description: fmt.Sprintf("Exodrive copies %s from %s to its old folder. It checks the copy, then removes the copy on the drive.",
				it.Title, from),
			Actions: []ui.AlertAction{
				{Text: "Cancel", Variant: ui.ButtonSecondary, TestID: "rollback-cancel",
					OnPress: func() { state.RollbackTarget.Set("") }},
				{Text: "Move back", Variant: ui.ButtonPrimary, TestID: "rollback-confirm",
					OnPress: func() { triggerRollback(state, id) }},
			},
			TestID: "rollback-dialog",
		}
	})
}

// confirmDelete asks before an unused cache leaves this Mac for good.
func confirmDelete(cx *core.Scope, state *AppState) core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		id := state.CleanTarget.Get()
		it, found := itemByID(state, id)
		description := "Exodrive deletes this cache from this Mac. The data is gone after this step."
		if found {
			description = fmt.Sprintf("Exodrive deletes %s from this Mac. The data is gone after this step.",
				shortPath(it.CurrentPath))
		}
		return ui.AlertDialog{
			Open: func() bool { return state.CleanTarget.Get() != "" },
			OnOpenChange: func(open bool, reason base.Reason) {
				if open {
					return
				}
				state.CleanTarget.Set("")
				if reason != base.ReasonSelect {
					state.Status.Set("Nothing deleted.")
				}
			},
			Title:       "Delete this cache?",
			Description: description,
			Actions: []ui.AlertAction{
				{Text: "Cancel", Variant: ui.ButtonSecondary, TestID: "clean-cancel",
					OnPress: func() { state.CleanTarget.Set("") }},
				{Text: "Delete cache", Variant: ui.ButtonPrimary, Color: func() core.Color { return ui.ColorsOf(cx).Danger() }, TestID: "clean-confirm",
					OnPress: func() { triggerClean(state, id) }},
			},
			TestID: "clean-dialog",
		}
	})
}
