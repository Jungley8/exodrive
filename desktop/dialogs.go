package desktop

import (
	"fmt"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/icons/lucide"
	"github.com/oneo-me/nsgo/ui"

	exoCore "exodrive/internal/core"
)

// renderDialogs stacks the confirmations and the report. Each one asks before
// data moves or leaves, and each one says what happens after.
func renderDialogs(cx *core.Scope, state *AppState) core.Node {
	return core.Group{
		confirmMove(cx, state),
		confirmMoveBack(cx, state),
		confirmKeepLocal(cx, state),
		confirmDelete(cx, state),
		reportDialog(cx, state),
	}
}

// confirmMove lists the caches that move, so a press never moves more than the
// person saw.
func confirmMove(cx *core.Scope, state *AppState) core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		colors := ui.ColorsOf(cx)
		t := state.Text
		selected := selectedItems(state)
		var total int64
		for _, it := range selected {
			total += it.SizeBytes
		}
		target := state.Target.Get()

		rows := make([]core.Node, 0, len(selected)+1)
		for i, it := range selected {
			if i == 5 {
				rows = append(rows, typeLine(fmt.Sprintf(t.AndMore, len(selected)-i), typeSmall, colors.Muted))
				break
			}
			rows = append(rows, core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
				core.Text{Content: it.Title, MaxLines: 1,
					Style:     core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), FontSize: 13, LineHeight: 18},
					StyleFunc: func() core.Style { return core.Style{Color: colors.Text()} }},
				typeLine(bytes(it.SizeBytes), figures(typeSmall), colors.Muted),
			}})
		}

		children := make([]core.Node, 0, 2)
		if target.SameVolume {
			children = append(children, noteLine(cx, lucide.Info, colors.Warn, "confirm-reminder", t.FolderOnThisMac))
		}
		children = append(children, core.View{Style: core.Style{Direction: core.Column, Gap: 4, Padding: core.Symmetric(8, 10), CornerRadius: 8},
			StyleFunc: ui.Filled(colors.Inset), Children: rows})

		return ui.Dialog{
			Open: func() bool { return state.ConfirmOpen.Get() },
			OnOpenChange: func(open bool, _ base.Reason) {
				if open {
					return
				}
				state.ConfirmOpen.Set(false)
			},
			Title:       fmt.Sprintf(t.MoveQuestion, t.counted(len(selected))),
			Description: fmt.Sprintf(t.MoveDescription, bytes(total), shortPath(target.Path)),
			Children:    children,
			Footer: []core.Node{
				ui.Button{Text: t.Cancel, Variant: ui.ButtonSecondary, TestID: "confirm-cancel",
					OnPress: func() { state.ConfirmOpen.Set(false); state.Status.Set(t.NothingMoved) }},
				ui.Button{Text: fmt.Sprintf(t.MoveButton, t.counted(len(selected))), Variant: ui.ButtonPrimary, TestID: "confirm-move",
					OnPress: func() { state.ConfirmOpen.Set(false); triggerMigration(state) }},
			},
			TestID: "confirm",
		}
	})
}

// confirmMoveBack asks before a cache returns to this Mac.
func confirmMoveBack(cx *core.Scope, state *AppState) core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		t := state.Text
		id := state.RollbackTarget.Get()
		it, _ := itemByID(state, id)
		from := t.FromTheDrive
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
					state.Status.Set(t.NothingMoved)
				}
			},
			Title:       t.MoveBackQuestion,
			Description: fmt.Sprintf(t.MoveBackDescription, it.Title, from),
			Actions: []ui.AlertAction{
				{Text: t.Cancel, Variant: ui.ButtonSecondary, TestID: "rollback-cancel",
					OnPress: func() { state.RollbackTarget.Set("") }},
				{Text: t.MoveBack, Variant: ui.ButtonPrimary, TestID: "rollback-confirm",
					OnPress: func() { triggerRollback(state, id) }},
			},
			TestID: "rollback-dialog",
		}
	})
}

// confirmKeepLocal asks before the copy on the drive goes. The folder on this
// Mac stays, so the dialog names the path that leaves.
func confirmKeepLocal(cx *core.Scope, state *AppState) core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		t := state.Text
		id := state.KeepLocalTarget.Get()
		it, _ := itemByID(state, id)
		return ui.AlertDialog{
			Open: func() bool { return state.KeepLocalTarget.Get() != "" },
			OnOpenChange: func(open bool, reason base.Reason) {
				if open {
					return
				}
				state.KeepLocalTarget.Set("")
				if reason != base.ReasonSelect {
					state.Status.Set(t.NothingMoved)
				}
			},
			Title:       t.KeepLocalQuestion,
			Description: fmt.Sprintf(t.KeepLocalDescription, shortPath(it.DestinationPath)),
			Actions: []ui.AlertAction{
				{Text: t.Cancel, Variant: ui.ButtonSecondary, TestID: "keep-local-cancel",
					OnPress: func() { state.KeepLocalTarget.Set("") }},
				{Text: t.KeepLocalAction, Variant: ui.ButtonPrimary, Color: func() core.Color { return ui.ColorsOf(cx).Danger() }, TestID: "keep-local-confirm",
					OnPress: func() { triggerKeepLocal(state, id) }},
			},
			TestID: "keep-local-dialog",
		}
	})
}

// confirmDelete asks before an unused cache leaves this Mac for good.
func confirmDelete(cx *core.Scope, state *AppState) core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		t := state.Text
		id := state.CleanTarget.Get()
		it, found := itemByID(state, id)
		description := t.DeleteDescriptionNoPath
		if found {
			description = fmt.Sprintf(t.DeleteDescription, shortPath(it.CurrentPath))
		}
		return ui.AlertDialog{
			Open: func() bool { return state.CleanTarget.Get() != "" },
			OnOpenChange: func(open bool, reason base.Reason) {
				if open {
					return
				}
				state.CleanTarget.Set("")
				if reason != base.ReasonSelect {
					state.Status.Set(t.NothingDeleted)
				}
			},
			Title:       t.DeleteQuestion,
			Description: description,
			Actions: []ui.AlertAction{
				{Text: t.Cancel, Variant: ui.ButtonSecondary, TestID: "clean-cancel",
					OnPress: func() { state.CleanTarget.Set("") }},
				{Text: t.DeleteCache, Variant: ui.ButtonPrimary, Color: func() core.Color { return ui.ColorsOf(cx).Danger() }, TestID: "clean-confirm",
					OnPress: func() { triggerClean(state, id) }},
			},
			TestID: "clean-dialog",
		}
	})
}

// reportDialog shows the exact text that a report carries. Nothing leaves this
// Mac until a person presses the issue button, and the browser opens the link.
func reportDialog(cx *core.Scope, state *AppState) core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		colors := ui.ColorsOf(cx)
		t := state.Text
		return ui.Dialog{
			Open: func() bool { return state.ReportOpen.Get() },
			OnOpenChange: func(open bool, _ base.Reason) {
				if !open {
					closeReport(state)
				}
			},
			Title:       t.ReportTitle,
			Description: t.ReportDescription,
			Children: []core.Node{
				core.View{
					Style:     core.Style{Direction: core.Column, Padding: core.Symmetric(8, 10), CornerRadius: 8},
					StyleFunc: ui.Filled(colors.Inset),
					Children: []core.Node{
						core.Text{
							Content:   state.ReportText.Get(),
							Style:     core.Style{FontSize: 11.5, LineHeight: 16, FontFamily: ui.Mono},
							StyleFunc: func() core.Style { return core.Style{Color: colors.Text()} },
							TestID:    "report-text",
						},
					},
				},
			},
			Footer: []core.Node{
				ui.Button{Text: t.Cancel, Variant: ui.ButtonSecondary, TestID: "report-cancel",
					OnPress: func() { closeReport(state) }},
				ui.Button{Text: t.ReportCopy, Variant: ui.ButtonSecondary, Icon: lucide.ClipboardCopy, TestID: "report-copy",
					OnPress: func() { copyReport(cx, state, state.ReportText.Get()) }},
				ui.Button{Text: t.ReportIssue, Variant: ui.ButtonPrimary, Icon: lucide.ExternalLink, TestID: "report-issue",
					OnPress: func() {
						records := exoCore.RecentFailures(reportRecords)
						cx.OpenURL(reportURL(reportTitle(records, t), state.ReportText.Get()), nil)
						closeReport(state)
					}},
			},
			TestID: "report",
		}
	})
}
