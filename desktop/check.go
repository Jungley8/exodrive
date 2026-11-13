package desktop

import (
	"fmt"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/icons/lucide"
	"github.com/oneo-me/nsgo/ui"

	exoCore "exodrive/internal/core"
)

// checkState is the answer of one check, as the card shows it.
type checkState struct {
	State exoCore.VerifyState
	Files int64
	Bytes int64
}

// triggerCheck reads a moved copy and compares it with the digest that the
// move recorded. It writes nothing to the drive and changes no record.
func triggerCheck(state *AppState, it exoCore.CacheItem) {
	if busy(state) {
		return
	}
	t := state.Text
	state.IsChecking.Set(true)
	state.CheckTarget.Set(it.ID)
	state.Status.Set(fmt.Sprintf(t.CheckingCopy, it.Title))

	// A cache that runs again drops its old answer, so the card never shows a
	// result that belongs to an earlier press.
	previous := make(map[string]checkState, len(state.Checks.GetUntracked()))
	for key, value := range state.Checks.GetUntracked() {
		previous[key] = value
	}
	delete(previous, it.ID)
	state.Checks.Set(previous)

	go func() {
		result, err := state.Engine.VerifyID(it.ID)
		state.Root.Post(func() {
			state.IsChecking.Set(false)
			state.CheckTarget.Set("")
			state.Status.Set("")
			if err != nil {
				// A check that cannot read the copy says so in the banner. The
				// card carries no failure block here, because its retry button
				// moves data, and a check moves none.
				recordFailure(actionCheck, it.ID, err)
				f := explain(err, actionCheck, t)
				state.Notice.Set(notice{Kind: noticeDanger, Title: f.Title, Body: f.Body, Hint: f.Hint})
				return
			}
			next := make(map[string]checkState, len(state.Checks.GetUntracked())+1)
			for key, value := range state.Checks.GetUntracked() {
				next[key] = value
			}
			next[it.ID] = checkState{State: result.State, Files: result.Files, Bytes: result.Bytes}
			state.Checks.Set(next)
		})
	}()
}

// checkCardLine shows the answer of the last check under the copy path, with
// the next step in words. A cache with no answer, and one that no check has
// run on, adds no line.
func checkCardLine(cx *core.Scope, state *AppState, it exoCore.CacheItem) core.Node {
	colors := ui.ColorsOf(cx)
	t := state.Text
	if state.CheckTarget.Get() == it.ID {
		return inlineNote(cx, lucide.Loader, colors.Muted, t.CheckRunning)
	}
	answer, ok := state.Checks.Get()[it.ID]
	if !ok {
		return nil
	}
	switch answer.State {
	case exoCore.VerifyOK:
		return inlineNote(cx, lucide.CircleCheck, colors.Up, t.CheckOK)
	case exoCore.VerifyChanged:
		return inlineNote(cx, lucide.CircleAlert, colors.Warn, t.CheckChanged)
	case exoCore.VerifyMissing:
		return inlineNote(cx, lucide.PlugZap, colors.Danger, t.CheckMissing)
	case exoCore.VerifyUnverified:
		return inlineNote(cx, lucide.Info, colors.Muted, t.CheckUnverified)
	}
	return nil
}
