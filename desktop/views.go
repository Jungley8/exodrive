package desktop

import (
	"fmt"
	"strings"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/icons/lucide"
	"github.com/oneo-me/nsgo/ui"

	exoCore "exodrive/internal/core"
)

// The window's type: a small scale, with the figures set in one width so they
// line up and do not jump while a move runs.
var (
	typeTitle = core.Style{FontSize: 20, LineHeight: 26, FontWeight: ui.Semibold}
	typeLead  = core.Style{FontSize: 15, LineHeight: 20, FontWeight: ui.Semibold}
	typeLabel = core.Style{FontSize: 13, LineHeight: 18, FontWeight: ui.Medium}
	typeSmall = core.Style{FontSize: 12, LineHeight: 16}
	typeMono  = core.Style{FontSize: 11.5, LineHeight: 16, FontFamily: ui.Mono}
)

// typeLine is one line of text in s, in color c.
func typeLine(content string, s core.Style, c func() core.Color) core.Text {
	return core.Text{Content: content, Style: s, StyleFunc: func() core.Style { return core.Style{Color: c()} }}
}

// typeLinef is one line of text whose content follows fn.
func typeLinef(fn func() string, s core.Style, c func() core.Color) core.Text {
	t := typeLine("", s, c)
	t.ContentFunc = fn
	return t
}

// figures sets a style's figures to one width.
func figures(s core.Style) core.Style {
	s.FontVariant = core.FontTabularNums
	return s
}

// fill grows to the free space of a row.
func fill() core.Node {
	return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}}
}

// panel is a surface on the page: a soft corner, a hairline rim and a calm
// background. The window stacks only a few, so a rim separates them without a
// shadow.
func panel(cx *core.Scope, children []core.Node) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{
		Style:     core.Style{Direction: core.Column, Gap: 12, Padding: core.Symmetric(14, 16), CornerRadius: 12, BorderCurve: core.BorderCurveContinuous, BorderWidth: 1},
		StyleFunc: func() core.Style { return core.Style{Background: colors.Surface(), BorderColor: colors.Border()} },
		Children:  children,
	}
}

func renderHeader(cx *core.Scope, state *AppState) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{
		Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, JustifyContent: core.JustifySpaceBetween, Gap: 16},
		Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Column, Gap: 1, FlexShrink: 1}, Children: []core.Node{
				typeLine("Exodrive", typeTitle, colors.Text),
				typeLine("Move developer caches to an external drive.", typeSmall, colors.Muted),
			}},
			core.Dynamic(func(cx *core.Scope) core.Node { return renderHeaderNote(cx, state) }),
		},
	}
}

// renderHeaderNote shows either that a check runs, or the promise that makes a
// move safe to try.
func renderHeaderNote(cx *core.Scope, state *AppState) core.Node {
	colors := ui.ColorsOf(cx)
	if state.IsScanning.Get() {
		return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
			ui.Spinner{Size: 14, Label: "Checking this Mac"},
			typeLine("Checking this Mac...", typeSmall, colors.Muted),
		}}
	}
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6, FlexShrink: 1}, Children: []core.Node{
		ui.Icon(lucide.ShieldCheck, 14, colors.Muted),
		typeLine("A cache frees its space only after the copy passes the check.", typeSmall, colors.Muted),
	}}
}

// renderDrives is the window's gauge: this Mac on the left, the external drive
// on the right, and the amount of cache that sits on each.
//
// The folder field stays outside the Dynamic regions, so typing a path never
// rebuilds the field under the pointer.
func renderDrives(cx *core.Scope, state *AppState) core.Node {
	return panel(cx, []core.Node{core.View{
		Style: core.Style{Direction: core.Row, AlignItems: core.AlignStretch, Gap: 18},
		Children: []core.Node{
			core.Dynamic(func(cx *core.Scope) core.Node {
				localBytes, _, _, _ := tally(state.Items.Get())
				return thisMacGauge(cx, state.Local.Get(), localBytes)
			}),
			driveLane(cx),
			core.View{
				Style: core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(0), Direction: core.Column, Gap: 8},
				Children: []core.Node{
					core.Dynamic(func(cx *core.Scope) core.Node { return externalGaugeHead(cx, state) }),
					targetInput(cx, state),
					core.Dynamic(func(cx *core.Scope) core.Node { return targetNote(cx, state) }),
				},
			},
		},
	}})
}

// barPart is one segment of a gauge: how many bytes it holds and its color.
type barPart struct {
	n     int64
	color func() core.Color
}

// gaugeBar draws the parts in order over a track that stands for the free
// space. A part too small to see keeps two points of width, so a little cache
// still shows.
func gaugeBar(cx *core.Scope, total int64, parts []barPart) core.Node {
	colors := ui.ColorsOf(cx)
	children := make([]core.Node, 0, len(parts))
	for _, part := range parts {
		if part.n <= 0 || total <= 0 {
			continue
		}
		pct := float32(part.n) / float32(total) * 100
		if pct > 100 {
			pct = 100
		}
		children = append(children, core.View{
			Style:     core.Style{Width: core.Pct(pct), MinWidth: core.Pt(2), Height: core.Pt(8)},
			StyleFunc: ui.Filled(part.color),
		})
	}
	return core.View{
		Style:     core.Style{Direction: core.Row, Height: core.Pt(8), CornerRadius: 4, Overflow: core.Hidden},
		StyleFunc: ui.Filled(colors.Track),
		Children:  children,
	}
}

// gaugeHead is a gauge's title on the left and its free space on the right.
func gaugeHead(cx *core.Scope, icon func() *core.SVG, title, value string) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6}, Children: []core.Node{
		ui.Icon(icon, 14, colors.Muted),
		typeLine(title, typeSmall, colors.Muted),
		fill(),
		typeLine(value, figures(typeLabel), colors.Text),
	}}
}

// gaugeNote is the colored note under a gauge: which part of the bar it is.
func gaugeNote(cx *core.Scope, c func() core.Color, content string) core.Node {
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6}, Children: []core.Node{
		core.View{Style: core.Style{Width: core.Pt(8), Height: core.Pt(8), CornerRadius: 4}, StyleFunc: ui.Filled(c)},
		typeLine(content, typeSmall, ui.ColorsOf(cx).Muted),
	}}
}

func thisMacGauge(cx *core.Scope, local diskState, cacheBytes int64) core.Node {
	colors := ui.ColorsOf(cx)
	head := "Free space unknown"
	if local.Known {
		head = bytes(local.Free) + " free"
	}
	note := "No caches found"
	if cacheBytes > 0 {
		note = bytes(cacheBytes) + " in caches"
	}
	other := local.Total - local.Free - cacheBytes
	if !local.Known || other < 0 {
		other = 0
	}
	return core.View{
		Style: core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(0), Direction: core.Column, Gap: 8},
		Children: []core.Node{
			gaugeHead(cx, lucide.Laptop, "This Mac", head),
			gaugeBar(cx, local.Total, []barPart{{cacheBytes, colors.Warn}, {other, colors.Muted}}),
			gaugeNote(cx, colors.Warn, note),
		},
	}
}

// driveLane is the arrow between the two places.
func driveLane(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{
		Style: core.Style{Width: core.Pt(36), Direction: core.Column, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
		Children: []core.Node{core.View{
			Style:     core.Style{Width: core.Pt(28), Height: core.Pt(28), CornerRadius: 14, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
			StyleFunc: ui.Filled(colors.Inset),
			Children:  []core.Node{ui.Icon(lucide.ArrowRight, 14, colors.Muted)},
		}},
	}
}

// externalGaugeHead is what the drive holds: its free space, its share of the
// bar, and the note that names the colored part.
func externalGaugeHead(cx *core.Scope, state *AppState) core.Node {
	colors := ui.ColorsOf(cx)
	target := state.Target.Get()
	moved := movedUnder(state.Items.Get(), target.Path)

	head, note := "No folder chosen", volumeHint()
	noteColor := colors.Muted
	total, other := int64(0), int64(0)
	switch {
	case target.Ready:
		head = bytes(target.Free) + " free"
		other = target.Total - target.Free - moved
		if other < 0 {
			other = 0
		}
		total = target.Total
		if moved > 0 {
			note, noteColor = bytes(moved)+" moved here", colors.Up
		} else {
			note = "No caches moved yet"
		}
	case target.Path != "":
		head = "Not ready"
		note = "The folder failed the check."
	}

	return core.View{
		Style: core.Style{Direction: core.Column, Gap: 8},
		Children: []core.Node{
			gaugeHead(cx, lucide.Usb, "External drive", head),
			gaugeBar(cx, total, []barPart{{moved, colors.Up}, {other, colors.Muted}}),
			gaugeNote(cx, noteColor, note),
		},
	}
}

// targetNote is the line under the folder field: the reason a folder fails the
// check, or the reminder that a folder on this Mac frees no space. The reminder
// never stops a move; it only says what a move cannot do.
func targetNote(cx *core.Scope, state *AppState) core.Node {
	colors := ui.ColorsOf(cx)
	target := state.Target.Get()
	switch {
	case target.Ready && target.SameVolume:
		return noteLine(cx, lucide.Info, colors.Warn, "target-reminder",
			"This folder is on this Mac. A move here frees no space.")
	case target.Path != "" && !target.Ready && target.Problem != "":
		return noteLine(cx, lucide.TriangleAlert, colors.Danger, "target-problem", target.Problem)
	}
	return core.Group{}
}

// noteLine is one line of advice under a field, with an icon and a color that
// follow how serious the advice is.
func noteLine(cx *core.Scope, icon func() *core.SVG, c func() core.Color, testID, content string) core.Node {
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignStart, Gap: 6}, Children: []core.Node{
		ui.Icon(icon, 12, c),
		core.Text{Content: content, TestID: testID,
			Style:     core.Style{FlexShrink: 1, FontSize: 12, LineHeight: 16},
			StyleFunc: func() core.Style { return core.Style{Color: c()} }},
	}}
}

// targetInput is the folder field: the path in a box, and the button that
// opens the folder picker.
func targetInput(cx *core.Scope, state *AppState) core.Node {
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
		core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: []core.Node{
			ui.Field{Input: core.Input{
				Style:       core.Style{Height: core.Pt(26), Padding: core.Symmetric(0, 8), FontSize: 12},
				Value:       func() string { return state.TargetDir.Get() },
				Placeholder: "Folder on the drive",
				OnChange: func(text string) {
					state.TargetDir.Set(strings.TrimSpace(text))
					triggerTargetCheck(state)
				},
				TestID: "target",
			}},
		}},
		ui.Button{
			Text: "Choose", Variant: ui.ButtonSecondary, Size: ui.SizeSmall, Icon: lucide.FolderOpen,
			OnPress: func() { openPicker(state) },
			TestID:  "browse",
		},
	}}
}

// volumeHint says whether an external volume is connected. Without one, the
// folder picker shows only the internal disk, so the window says so first.
func volumeHint() string {
	volumes := exoCore.ExternalVolumes()
	switch len(volumes) {
	case 0:
		return "No external drive is connected."
	case 1:
		return volumes[0] + " is connected."
	default:
		return fmt.Sprintf("%d drives are connected.", len(volumes))
	}
}

// renderAction is the one place where the window says what to do next. The
// button always names that step: choose a folder, select caches, or move them.
func renderAction(cx *core.Scope, state *AppState) core.Node {
	if state.IsMigrating.Get() {
		return panel(cx, []core.Node{progressRow(cx, state)})
	}

	target := state.Target.Get()
	visible := visibleItems(state)
	selected := selectedItems(state)
	_, _, _, canMove := tally(visible)
	var total int64
	for _, it := range selected {
		total += it.SizeBytes
	}

	title, helper := actionCopy(target, len(visible), len(selected), canMove, total)
	colors := ui.ColorsOf(cx)
	children := []core.Node{
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 16}, Children: []core.Node{
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Direction: core.Column, Gap: 1}, Children: []core.Node{
				typeLine(title, typeLead, colors.Text),
				typeLine(helper, typeSmall, colors.Muted),
			}},
			actionButton(state, target, len(selected), canMove, len(visible)),
		}},
	}
	if n := state.Notice.Get(); n.shown() {
		children = append(children, core.View{Style: core.Style{Height: core.Pt(1)}, StyleFunc: ui.Filled(colors.Line)}, noticeBlock(cx, n))
	}
	return panel(cx, children)
}

// actionCopy names the next step and what it will do.
func actionCopy(target targetState, shown, selected, canMove int, total int64) (title, helper string) {
	switch {
	case !target.Ready && target.Problem != "":
		return "Choose another folder", "Exodrive checked the folder and cannot use it."
	case !target.Ready:
		return "Choose a folder for the caches", "The caches move to this folder. Keep the drive connected."
	case shown == 0:
		return "No caches in this view", "Choose All, or another group in the list on the left."
	case canMove == 0:
		return "Nothing can move now", "Every cache is already on the drive, or it cannot move."
	case selected == 0:
		return "Select the caches to move", "A cache stays on this Mac until the copy passes the check."
	default:
		return "Move " + count(selected, "cache", "caches"),
			fmt.Sprintf("%s moves to the drive. Exodrive checks each copy before it frees space.", bytes(total))
	}
}

func actionButton(state *AppState, target targetState, selected, canMove, shown int) core.Node {
	disabled := func() bool { return busy(state) }
	switch {
	case !target.Ready:
		return ui.Button{Text: "Choose folder", Variant: ui.ButtonPrimary, Icon: lucide.FolderOpen,
			Disabled: disabled, OnPress: func() { openPicker(state) }, TestID: "choose-target"}
	case canMove == 0 || shown == 0:
		return core.View{}
	case selected == 0:
		return ui.Button{Text: fmt.Sprintf("Select all %d", canMove), Variant: ui.ButtonPrimary, Icon: lucide.Check,
			Disabled: disabled, OnPress: func() { selectAll(state, true) }, TestID: "select-all"}
	default:
		return ui.Button{Text: "Move " + count(selected, "cache", "caches"), Variant: ui.ButtonPrimary, Icon: lucide.ArrowRight,
			Disabled: disabled, OnPress: func() { state.ConfirmOpen.Set(true) }, TestID: "migrate"}
	}
}

// noticeBlock explains a failure: what stopped, what stays safe, and the next
// step. The window keeps it until the next action starts.
func noticeBlock(cx *core.Scope, n notice) core.Node {
	colors := ui.ColorsOf(cx)
	tints := ui.TintsOf(cx)
	accent, background, icon := colors.Accent, tints.Accent, lucide.Info
	switch n.Kind {
	case noticeDanger:
		accent, background, icon = colors.Danger, tints.Danger, lucide.TriangleAlert
	case noticeWarn:
		accent, background, icon = colors.Warn, tints.Warn, lucide.TriangleAlert
	}
	lines := []core.Node{
		typeLine(n.Title, typeLabel, accent),
	}
	if n.Body != "" {
		lines = append(lines, typeLine(n.Body, typeSmall, colors.Text))
	}
	if n.Hint != "" {
		lines = append(lines, typeLine(n.Hint, typeSmall, colors.Muted))
	}
	return core.View{
		Style:     core.Style{Direction: core.Row, AlignItems: core.AlignStart, Gap: 10, Padding: core.Symmetric(10, 12), CornerRadius: 8},
		StyleFunc: ui.Filled(background),
		TestID:    "notice",
		Children: []core.Node{
			ui.Icon(icon, 14, accent),
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Direction: core.Column, Gap: 2}, Children: lines},
		},
	}
}

// progressRow names the step, the cache and the share done, so a long move
// never looks frozen.
func progressRow(cx *core.Scope, state *AppState) core.Node {
	colors := ui.ColorsOf(cx)
	total := state.MoveTotal.Get()
	done := state.MoveDone.Get()
	current := state.MoveCurrent.Get()
	pct := state.ProgressPct.Get()
	stopping := state.CancelRequested.Get()

	title := "Moving " + current
	if state.MovePhase.Get() == exoCore.PhaseVerify {
		title = "Checking the copy of " + current
	}
	detail := fmt.Sprintf("Cache %d of %d. %.0f%% done.", done+1, total, pct*100)
	if total == 0 {
		title, detail = "Moving the caches", ""
	}
	if stopping {
		title, detail = "Finishing "+current, "Stopping after this cache."
	}

	text := []core.Node{
		typeLine(title, typeLabel, colors.Text),
		ui.Progress{Value: func() float32 { return pct * 100 }, Label: title},
	}
	if detail != "" {
		text = append(text, typeLinef(func() string { return detail }, typeSmall, colors.Muted))
	}
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 16}, Children: []core.Node{
		core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Direction: core.Column, Gap: 6}, Children: text},
		ui.Button{Text: "Stop after this cache", Variant: ui.ButtonSecondary, Icon: lucide.CircleX,
			Disabled: func() bool { return state.CancelRequested.Get() },
			OnPress:  func() { state.CancelRequested.Set(true) },
			TestID:   "cancel-move"},
	}}
}

// renderListHeader names the list and holds its filters: the place, the
// selection and the check.
func renderListHeader(cx *core.Scope, state *AppState) core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		colors := ui.ColorsOf(cx)
		visible := visibleItems(state)
		_, _, _, canMove := tally(visible)
		selected := len(selectedItems(state))

		children := []core.Node{
			typeLine("Caches", typeLabel, colors.Text),
			typeLine(fmt.Sprintf("%d", len(visible)), figures(typeSmall), colors.Muted),
			fill(),
			placeFilter(cx, state),
		}
		if selected > 0 {
			children = append(children, typeLine(fmt.Sprintf("%d selected", selected), figures(typeSmall), colors.Muted))
			children = append(children, ui.Button{Text: "Clear", Variant: ui.ButtonGhost, Size: ui.SizeSmall,
				OnPress: func() { selectAll(state, false) }, TestID: "clear-selection"})
		}
		if selected > 0 && selected < canMove {
			children = append(children, ui.Button{Text: fmt.Sprintf("Select all %d", canMove), Variant: ui.ButtonGhost, Size: ui.SizeSmall,
				OnPress: func() { selectAll(state, true) }, TestID: "select-all-rest"})
		}
		children = append(children, ui.Button{Text: "Rescan", Variant: ui.ButtonSecondary, Size: ui.SizeSmall, Icon: lucide.RefreshCw,
			Disabled: func() bool { return busy(state) }, OnPress: func() { triggerScan(state) }, TestID: "rescan"})

		return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: children}
	})
}

// placeFilter keeps one place in the list: this Mac, the drive, or both. The
// words match the state on every card, so a press reads as a filter on what
// the cards already say.
func placeFilter(cx *core.Scope, state *AppState) core.Node {
	return ui.ToggleGroup{Label: "Place", TestID: "place-filter", Children: []core.Node{
		placeToggle(state, placeAll, "All", "place-all"),
		placeToggle(state, placeLocal, "On this Mac", "place-this-mac"),
		placeToggle(state, placeMoved, "On the drive", "place-drive"),
	}}
}

func placeToggle(state *AppState, place, text, testID string) core.Node {
	return ui.ToggleButton{Text: text, TestID: testID,
		On:       func() bool { return state.FilterPlace.Get() == place },
		OnChange: func(bool) { setPlace(state, place) }}
}

// renderListArea is the rail of groups beside the cards of the chosen group.
// The rail keeps every group visible, so the window always says what it found.
func renderListArea(cx *core.Scope, state *AppState) core.Node {
	return core.View{
		Style: core.Style{Direction: core.Row, AlignItems: core.AlignStretch, Gap: 14,
			FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)},
		Children: []core.Node{
			core.Dynamic(func(cx *core.Scope) core.Node { return renderRail(cx, state) }),
			core.View{
				Style: core.Style{Direction: core.Column, Gap: 8, FlexGrow: 1, FlexShrink: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(0), MinHeight: core.Pt(0)},
				Children: []core.Node{
					core.Dynamic(func(cx *core.Scope) core.Node { return renderListHeader(cx, state) }),
					core.Dynamic(func(cx *core.Scope) core.Node { return renderList(cx, state) }),
				},
			},
		},
	}
}

// renderRail shows what the check found, broken down by group. A bar under
// each name shows how much of that group sits on this Mac and how much already
// moved, so a press on a group reads before it filters.
func renderRail(cx *core.Scope, state *AppState) core.Node {
	colors := ui.ColorsOf(cx)
	items := state.Items.Get()
	stats := families(items)
	if len(stats) == 0 {
		return core.Group{}
	}
	local, moved, unused, _ := tally(items)
	all := familyStat{Family: placeAllFamily, Count: len(items), Local: local + unused, Moved: moved}
	total := all.Local + all.Moved

	children := []core.Node{typeLine("Detected", typeLabel, colors.Text),
		railRow(cx, state, all, total, "All caches", "rail-all")}
	for _, stat := range stats {
		children = append(children, railRow(cx, state, stat, total, string(stat.Family), "rail-"+string(stat.Family)))
	}

	return core.View{
		Style: core.Style{Width: core.Pt(176), FlexShrink: 0, MinHeight: core.Pt(0), Direction: core.Column, Gap: 2,
			Padding: core.Symmetric(12, 8), CornerRadius: 12, BorderCurve: core.BorderCurveContinuous, BorderWidth: 1},
		StyleFunc: func() core.Style { return core.Style{Background: colors.Surface(), BorderColor: colors.Border()} },
		TestID:    "rail",
		Children: []core.Node{core.Scroll{
			Style:    core.Style{Direction: core.Column, Gap: 2, FlexGrow: 1, MinHeight: core.Pt(0)},
			Children: children,
		}},
	}
}

// placeAllFamily is the rail's first row: every group at once.
const placeAllFamily cacheFamily = ""

// railRow is one group: its name, how many caches it holds, and its share of
// the caches with a bar split by place.
func railRow(cx *core.Scope, state *AppState, stat familyStat, total int64, label, testID string) core.Node {
	colors := ui.ColorsOf(cx)
	chosen := state.FilterFamily.Get() == stat.Family
	ink := colors.Muted
	if chosen {
		ink = colors.Text
	}
	return core.View{
		Style: core.Style{Direction: core.Column, Gap: 5, Padding: core.Symmetric(7, 8), CornerRadius: 8},
		StyleFunc: func() core.Style {
			if chosen {
				return core.Style{Background: colors.Inset()}
			}
			return core.Style{}
		},
		Role: core.RoleButton,
		OnPress: func(core.PressEvent) {
			if chosen && stat.Family != placeAllFamily {
				setFamily(state, placeAllFamily)
				return
			}
			setFamily(state, stat.Family)
		},
		LabelFunc: func() string { return fmt.Sprintf("%s, %s", label, count(stat.Count, "cache", "caches")) },
		TestID:    testID,
		Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6}, Children: []core.Node{
				typeLine(label, typeSmall, ink),
				fill(),
				typeLine(fmt.Sprintf("%d", stat.Count), figures(typeSmall), colors.Faint),
			}},
			gaugeBar(cx, total, []barPart{{stat.Local, colors.Warn}, {stat.Moved, colors.Up}}),
		},
	}
}

func renderList(cx *core.Scope, state *AppState) core.Node {
	visible := visibleItems(state)
	if len(visible) == 0 {
		return emptyList(cx, state)
	}
	return core.Scroll{
		Style: core.Style{Direction: core.Column, Gap: 8, FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)},
		Children: []core.Node{
			core.Each(
				func() []exoCore.CacheItem { return visibleItems(state) },
				func(it exoCore.CacheItem) string { return it.ID },
				func(cx *core.Scope, item func() exoCore.CacheItem, _ func() int) core.Node {
					return core.Dynamic(func(cx *core.Scope) core.Node { return renderRow(cx, state, item) })
				},
			),
		},
	}
}

// emptyList says what a check looks for, or that the filter hides every cache.
// A failed check already explains itself in the notice, so the list stays bare
// then.
func emptyList(cx *core.Scope, state *AppState) core.Node {
	colors := ui.ColorsOf(cx)
	if state.Notice.Get().Kind == noticeDanger {
		return core.View{Style: core.Style{FlexGrow: 1, MinHeight: core.Pt(0)}}
	}
	if len(state.Items.Get()) > 0 {
		return core.View{
			Style: core.Style{FlexGrow: 1, MinHeight: core.Pt(0), Direction: core.Column, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: 4},
			Children: []core.Node{
				ui.Icon(lucide.Funnel, 22, colors.Faint),
				typeLine("No caches in this view", typeLead, colors.Text),
				typeLine("Choose All, or another group in the list on the left.", typeSmall, colors.Muted),
			},
		}
	}
	return core.View{
		Style: core.Style{FlexGrow: 1, MinHeight: core.Pt(0), Direction: core.Column, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: 4},
		Children: []core.Node{
			ui.Icon(lucide.Package, 22, colors.Faint),
			typeLine("No caches found", typeLead, colors.Text),
			typeLine("Exodrive checks npm, pnpm, yarn, cargo, Go, bun, uv, pip, and Docker.", typeSmall, colors.Muted),
		},
	}
}

// renderAdvanced holds the few facts a developer wants once, kept out of the
// way of everyone else: the record file and what a move back needs.
func renderAdvanced(cx *core.Scope, state *AppState) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{
		Style:     core.Style{Direction: core.Column, Gap: 4, Padding: core.Symmetric(10, 14), CornerRadius: 10, BorderWidth: 1},
		StyleFunc: func() core.Style { return core.Style{Background: colors.Inset(), BorderColor: colors.Border()} },
		TestID:    "advanced",
		Children: []core.Node{
			typeLine("Move records", typeLabel, colors.Text),
			typeLine(shortPath(exoCore.GetManifestPath()), typeMono, colors.Muted),
			typeLine("A move back needs the record file and the drive. Keep both.", typeSmall, colors.Muted),
		},
	}
}

func renderFooter(cx *core.Scope, state *AppState) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
		typeLinef(func() string { return state.Status.Get() }, typeSmall, colors.Muted),
		fill(),
		core.Show{
			When: func() bool { return state.AdvancedOpen.Get() },
			Then: func(cx *core.Scope) core.Node {
				return ui.Button{Text: "Hide details", Variant: ui.ButtonGhost, Size: ui.SizeSmall, Icon: lucide.ChevronDown,
					OnPress: func() { state.AdvancedOpen.Set(false) }, TestID: "advanced-toggle"}
			},
			Else: func(cx *core.Scope) core.Node {
				return ui.Button{Text: "Details", Variant: ui.ButtonGhost, Size: ui.SizeSmall, Icon: lucide.ChevronRight,
					OnPress: func() { state.AdvancedOpen.Set(true) }, TestID: "advanced-toggle"}
			},
		},
	}}
}
