package desktop

import (
	"fmt"
	"os"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/icons/lucide"
	"github.com/oneo-me/nsgo/ui"

	exoCore "exodrive/internal/core"
)

// welcomeFile marks that the person read the first-run card. It sits beside the
// ledger, so a test that moves the ledger moves this file too.
const welcomeFile = "welcome"

// welcomeSeen reports whether the window showed the first-run card before. A
// folder that cannot be read counts as unseen: the card costs one press.
func welcomeSeen() bool {
	_, err := os.Stat(exoCore.StatePath(welcomeFile))
	return err == nil
}

// markWelcomeSeen records the card as read. A write that fails makes the card
// come back at the next start; it never stops the window.
func markWelcomeSeen() {
	_ = os.WriteFile(exoCore.StatePath(welcomeFile), []byte("seen\n"), 0o644)
}

// renderWelcome is the first-run card: the three steps in the order a person
// takes them, and the one action that starts the first step.
func renderWelcome(cx *core.Scope, state *AppState) core.Node {
	colors := ui.ColorsOf(cx)
	t := state.Text
	steps := []string{t.WelcomeStepOne, t.WelcomeStepTwo, t.WelcomeStepThree}
	rows := make([]core.Node, 0, len(steps))
	for i, step := range steps {
		rows = append(rows, core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignStart, Gap: 8}, Children: []core.Node{
			core.View{Style: core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 9, AlignItems: core.AlignCenter,
				JustifyContent: core.JustifyCenter, FlexShrink: 0}, StyleFunc: ui.Filled(colors.Inset),
				Children: []core.Node{typeLine(fmt.Sprint(i+1), figures(typeSmall), colors.Muted)}},
			typeLine(step, typeSmall, colors.Text),
		}})
	}

	return panel(cx, []core.Node{
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
			ui.Icon(lucide.Sparkles, 15, colors.Accent),
			typeLine(t.WelcomeTitle, typeLabel, colors.Text),
		}},
		core.View{Style: core.Style{Direction: core.Column, Gap: 6}, Children: rows},
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
			fill(),
			ui.Button{Text: t.WelcomeSkip, Variant: ui.ButtonGhost, Size: ui.SizeSmall,
				OnPress: func() { closeWelcome(state) }, TestID: "welcome-skip"},
			ui.Button{Text: t.WelcomeStart, Variant: ui.ButtonPrimary, Size: ui.SizeSmall, Icon: lucide.FolderOpen,
				OnPress: func() { closeWelcome(state); openPicker(state) }, TestID: "welcome-start"},
		}},
	})
}

// interruptedNotice reports the action that a closed window left running. The
// record is removed here, because this start owns the next one.
func interruptedNotice(t text) (notice, bool) {
	f, ok := exoCore.ReadInFlight()
	if !ok {
		return notice{}, false
	}
	_ = exoCore.ClearInFlight()
	body := t.InterruptedBodyAny
	if f.Title != "" {
		body = fmt.Sprintf(t.InterruptedBody, f.Title)
	}
	return notice{Kind: noticeWarn, Title: t.InterruptedTitle, Body: body, Hint: t.InterruptedHint}, true
}

// closeWelcome hides the card and records it, so the next start opens on the
// work itself.
func closeWelcome(state *AppState) {
	if !state.WelcomeOpen.Get() {
		return
	}
	state.WelcomeOpen.Set(false)
	markWelcomeSeen()
}
