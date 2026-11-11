package desktop

import (
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/ui"
)

// Theme is the window's look: graphite surfaces, a violet accent, and two
// state colors that carry one meaning each. Amber marks caches that still take
// space on this Mac. Teal marks caches that already moved to the external
// drive. Every other color stays neutral, so a state color always means state.
func Theme() *core.Theme {
	t := ui.DefaultTheme()

	lightInk := core.RGB(0x1a1d23)
	t.Light = core.Palette{
		Background:     core.RGB(0xf4f5f7),
		Surface:        core.RGB(0xffffff),
		SurfaceHover:   lightInk.Faded(0.05),
		SurfacePressed: lightInk.Faded(0.09),
		Border:         core.RGB(0xdfe3e9),
		Text:           lightInk,
		TextMuted:      core.RGB(0x5d6470),
		Accent:         core.RGB(0x5a4bc9),
		AccentHover:    core.RGB(0x5041b7),
		AccentPressed:  core.RGB(0x4638a4),
		AccentText:     core.RGB(0xffffff),
		Danger:         core.RGB(0xb93b3b),
		Warning:        core.RGB(0xb4690e),
		Success:        core.RGB(0x16705c),
		Selection:      core.RGBA(0x5a4bc9, 0.20),
		Focus:          core.RGBA(0x5a4bc9, 0.55),
		Scrollbar:      lightInk.Faded(0.28),
		Colors: map[string]core.Color{
			"inset": core.RGB(0xeceef2), "raised": core.RGB(0xffffff),
			"field": core.RGB(0xfafbfc), "fieldBorder": core.RGB(0xd6dbe2),
			"line": lightInk.Faded(0.09), "faint": core.RGB(0x949ba6),
			"accentFill": core.RGB(0x5a4bc9),
			"light":      core.RGB(0x1a1d23), "onLight": core.RGB(0xffffff),
			"track": core.RGB(0xe3e6eb), "knob": core.RGB(0xffffff),
			"code": core.RGB(0x2c3038), "info": core.RGB(0x0e7490),
			"down": core.RGB(0xb4444c),
		},
	}

	darkInk := core.RGB(0xe8eaee)
	t.Dark = core.Palette{
		Background:     core.RGB(0x121419),
		Surface:        core.RGB(0x1b1e25),
		SurfaceHover:   darkInk.Faded(0.06),
		SurfacePressed: darkInk.Faded(0.10),
		Border:         core.RGB(0x2e333d),
		Text:           darkInk,
		TextMuted:      core.RGB(0x9ba2ad),
		Accent:         core.RGB(0xa99bf5),
		AccentHover:    core.RGB(0xb9adf8),
		AccentPressed:  core.RGB(0x9789e8),
		AccentText:     core.RGB(0x17132b),
		Danger:         core.RGB(0xe58f95),
		Warning:        core.RGB(0xe0ac5c),
		Success:        core.RGB(0x63c3a6),
		Selection:      core.RGBA(0xa99bf5, 0.24),
		Focus:          core.RGBA(0xa99bf5, 0.80),
		Scrollbar:      darkInk.Faded(0.32),
		Colors: map[string]core.Color{
			"inset": core.RGB(0x171a20), "raised": core.RGB(0x242831),
			"field": core.RGB(0x161920), "fieldBorder": core.RGB(0x363c47),
			"line": darkInk.Faded(0.08), "faint": core.RGB(0x707784),
			"accentFill": core.RGB(0x8e7ff0),
			"light":      core.RGB(0xe8eaee), "onLight": core.RGB(0x121419),
			"track": core.RGB(0x2a2f38), "knob": core.RGB(0xf2f3f6),
			"code": core.RGB(0xdde2ea), "info": core.RGB(0x67c3d8),
			"down": core.RGB(0xe58f95),
		},
	}

	t.Radius = 10
	return t
}
