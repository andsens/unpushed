package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"sync"

	"fyne.io/fyne/v2"
)

// Look is everything an icon shows. Repo.Look decides it, Color and draw
// turn it into pixels.
type Look struct {
	Up     bool // Unpushed commits
	Down   bool // Unpulled commits
	Failed bool // The last push or pull failed
}

var (
	green  = color.NRGBA{0x43, 0xa0, 0x47, 0xff}
	yellow = color.NRGBA{0xfb, 0xc0, 0x2d, 0xff}
	red    = color.NRGBA{0xe5, 0x39, 0x35, 0xff}
	white  = color.NRGBA{0xff, 0xff, 0xff, 0xff}
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

const iconSize = 64

// A shape takes coordinates from -1 to 1, y pointing down.
type shape func(x, y float64) bool

type layer struct {
	color color.NRGBA
	shape shape
}

func circle(r float64) shape {
	return func(x, y float64) bool { return x*x+y*y <= r*r }
}

// arrow is centred on cx, width is the head's half width.
func arrow(cx float64, up bool, width float64) shape {
	return func(x, y float64) bool {
		if !up {
			y = -y
		}
		// Pointing up: the head from the tip at -0.6 to -0.05, the stem on to 0.55
		dx := math.Abs(x - cx)
		head := y >= -0.6 && y <= -0.05 && dx <= width*(y+0.6)/0.55
		stem := y > -0.05 && y <= 0.55 && dx <= width*0.35
		return head || stem
	}
}

func (l Look) layers() []layer {
	layers := []layer{{l.Color(), circle(0.92)}}
	switch {
	case l.Up && l.Down:
		layers = append(layers, layer{white, arrow(-0.3, true, 0.24)}, layer{white, arrow(0.3, false, 0.24)})
	case l.Up:
		layers = append(layers, layer{white, arrow(0, true, 0.34)})
	case l.Down:
		layers = append(layers, layer{white, arrow(0, false, 0.34)})
	}
	return layers
}

// draw paints the layers in order, antialiased by 4x4 supersampling.
func draw(layers []layer) []byte {
	const samples = 4
	img := image.NewNRGBA(image.Rect(0, 0, iconSize, iconSize))
	for py := range iconSize {
		for px := range iconSize {
			var r, g, b, a float64
			for _, l := range layers {
				hits := 0
				for sy := range samples {
					for sx := range samples {
						x := (float64(px)+(float64(sx)+0.5)/samples)/iconSize*2 - 1
						y := (float64(py)+(float64(sy)+0.5)/samples)/iconSize*2 - 1
						if l.shape(x, y) {
							hits++
						}
					}
				}
				// Source over, in premultiplied alpha
				cov := float64(hits) / samples / samples * float64(l.color.A) / 255
				r = float64(l.color.R)*cov + r*(1-cov)
				g = float64(l.color.G)*cov + g*(1-cov)
				b = float64(l.color.B)*cov + b*(1-cov)
				a = cov + a*(1-cov)
			}
			if a > 0 {
				img.SetNRGBA(px, py, color.NRGBA{uint8(r / a), uint8(g / a), uint8(b / a), uint8(a * 255)})
			}
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img) // Encoding to memory doesn't fail
	return buf.Bytes()
}

// appIcon is the settings window's: both arrows on green, outside the scheme.
var appIcon = func() fyne.Resource {
	layers := Look{Up: true, Down: true}.layers()
	layers[0].color = green
	return fyne.NewStaticResource("unpushed.png", draw(layers))
}()

var iconCache sync.Map

func icon(l Look) fyne.Resource {
	if res, ok := iconCache.Load(l); ok {
		return res.(fyne.Resource)
	}
	res := fyne.NewStaticResource(fmt.Sprintf("unpushed-%t-%t-%t.png", l.Up, l.Down, l.Failed), draw(l.layers()))
	iconCache.Store(l, res)
	return res
}
