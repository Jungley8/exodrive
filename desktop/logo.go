package desktop

import (
	"time"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/icons/lucide"
	"github.com/oneo-me/nsgo/ui"
)

// markSize is the mark's side beside the wordmark.
const markSize core.Points = 30

// WindowTitle is the name the system shows for the window and in the Dock.
const WindowTitle = "Exodrive"

// brandMark is the window's mark: one plate that runs from the accent to the
// drive's color, and the arrow that leaves it. The plate carries the whole
// story in two colors, the same two the gauges use: this Mac, then the drive.
//
// The arrow moves only while a long action runs. Motion means work, so a
// window at rest holds still.
func brandMark(cx *core.Scope, state *AppState) core.Node {
	return core.Show{
		When: func() bool { return busy(state) },
		Then: func(cx *core.Scope) core.Node { return movingMark(cx, markSize) },
		Else: func(cx *core.Scope) core.Node { return stillMark(cx, markSize) },
	}
}

// stillMark is the mark at rest.
func stillMark(cx *core.Scope, size core.Points) core.Node {
	return markPlate(cx, size, "brand-mark", ui.Icon(lucide.ArrowRight, size*0.52, inkOnMark(cx)))
}

// movingMark is the mark while work runs: the arrow leaves the plate and fades,
// once every second and a half. The fade hides the loop's return to the start.
func movingMark(cx *core.Scope, size core.Points) core.Node {
	out := core.NewAnimatedValue(cx, 0)
	if !ui.ReduceMotion(cx) {
		cx.StartAnimation(core.Loop(core.Timing(out, 1, core.TimingConfig{
			Duration: 1500 * time.Millisecond,
			Easing:   core.BezierEasing(core.EaseInOut),
		}), 0), nil)
	}
	arrow := core.View{
		Style: core.Style{Animated: &core.AnimatedStyle{
			TranslateX: ui.Lerp(out, 0, float64(size*0.35)),
			Opacity:    ui.Lerp(out, 1, 0),
		}},
		Children: []core.Node{ui.Icon(lucide.ArrowRight, size*0.52, inkOnMark(cx))},
	}
	return markPlate(cx, size, "brand-mark-moving", arrow)
}

// markPlate draws the square the arrow sits on.
func markPlate(cx *core.Scope, size core.Points, testID string, children core.Node) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{
		Style: core.Style{Width: core.Pt(size), Height: core.Pt(size), FlexShrink: 0, Overflow: core.Hidden,
			AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
			CornerRadius: size * 0.3, BorderCurve: core.BorderCurveContinuous},
		StyleFunc: func() core.Style {
			return core.Style{Gradients: []core.Gradient{core.LinearGradient(135, colors.Accent(), colors.Up())}}
		},
		TestID:   testID,
		Children: []core.Node{children},
	}
}

// inkOnMark is the arrow's color. The plate is dark in a light window and pale
// in a dark one, so the arrow takes the accent's own text color both ways.
func inkOnMark(cx *core.Scope) func() core.Color {
	colors := ui.ColorsOf(cx)
	return colors.OnAccent
}
