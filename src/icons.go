package main

import (
	"fmt"
	"image/color"
	"sync"

	"fyne.io/fyne/v2"
)

// Look is everything an icon shows. Repo.Look decides it, Color and svg turn
// it into an image.
type Look struct {
	Up     bool // Unpushed commits
	Down   bool // Unpulled commits
	Failed bool // The last push or pull failed
}

var (
	green  = color.NRGBA{0x43, 0xa0, 0x47, 0xff}
	yellow = color.NRGBA{0xfb, 0xc0, 0x2d, 0xff}
	red    = color.NRGBA{0xe5, 0x39, 0x35, 0xff}
)

func (l Look) Color() color.NRGBA {
	switch {
	case l.Failed || l.Up && l.Down:
		return red
	case l.Up || l.Down:
		return yellow
	}
	return green
}

// Union is the look of several repos together, for the tray icon.
func (l Look) Union(o Look) Look {
	return Look{Up: l.Up || o.Up, Down: l.Down || o.Down, Failed: l.Failed || o.Failed}
}

// svg is the icon for l in the colour c, on a 64x64 canvas.
func (l Look) svg(c color.NRGBA) []byte {
	var marks string
	switch {
	case l.Failed:
		marks = `<path d="M18.6 18.6 L45.4 45.4 M45.4 18.6 L18.6 45.4" stroke="white" stroke-width="7"/>`
	case l.Up && l.Down:
		marks = arrow(22.4, 7.7, true) + arrow(41.6, 7.7, false)
	case l.Up:
		marks = arrow(32, 10.9, true)
	case l.Down:
		marks = arrow(32, 10.9, false)
	}
	return fmt.Appendf(nil, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64">`+
		`<circle cx="32" cy="32" r="29.4" fill="#%02x%02x%02x"/><g fill="white">%s</g></svg>`,
		c.R, c.G, c.B, marks)
}

// arrow is centred on x with a head w to each side. Up and down arrows span
// the same height, so a pair lines up.
func arrow(x, w float64, up bool) string {
	tip, head, tail := 12.8, 30.4, 51.2
	if !up {
		tip, head, tail = 51.2, 33.6, 12.8
	}
	s := w * 0.35 // the stem's half width
	return fmt.Sprintf(`<path d="M%.1f %.1f L%.1f %.1f L%.1f %.1f L%.1f %.1f L%.1f %.1f L%.1f %.1f L%.1f %.1f Z"/>`,
		x, tip, x+w, head, x+s, head, x+s, tail, x-s, tail, x-s, head, x-w, head)
}

var iconCache sync.Map

func icon(l Look) fyne.Resource {
	if res, ok := iconCache.Load(l); ok {
		return res.(fyne.Resource)
	}
	res := fyne.NewStaticResource(fmt.Sprintf("unpushed-%t-%t-%t.svg", l.Up, l.Down, l.Failed), l.svg(l.Color()))
	iconCache.Store(l, res)
	return res
}

// appIcon is the app's own: both arrows on green, outside the scheme.
var appIcon = fyne.NewStaticResource("unpushed.svg", Look{Up: true, Down: true}.svg(green))
