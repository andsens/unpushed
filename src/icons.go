package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

var icons = map[State][]byte{
	Synced: led(color.NRGBA{0x43, 0xa0, 0x47, 0xff}),
	Behind: led(color.NRGBA{0xfb, 0xc0, 0x2d, 0xff}),
	Ahead:  led(color.NRGBA{0xe5, 0x39, 0x35, 0xff}),
	Failed: cross(color.NRGBA{0xe5, 0x39, 0x35, 0xff}),
}

const iconSize = 64

// draw renders an antialiased shape given its signed distance (in pixels) at
// each point, negative inside.
func draw(c color.NRGBA, dist func(x, y float64) float64) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, iconSize, iconSize))
	for y := range iconSize {
		for x := range iconSize {
			d := dist(float64(x)+0.5-iconSize/2, float64(y)+0.5-iconSize/2)
			px := c
			px.A = uint8(float64(c.A) * math.Max(0, math.Min(1, 0.5-d)))
			img.SetNRGBA(x, y, px)
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img) // Encoding to memory doesn't fail
	return buf.Bytes()
}

func led(c color.NRGBA) []byte {
	return draw(c, func(x, y float64) float64 { return math.Hypot(x, y) - iconSize*0.4 })
}

func cross(c color.NRGBA) []byte {
	return draw(c, func(x, y float64) float64 {
		// Distance to the nearer diagonal, limited to the circle the LED fills
		d := math.Min(math.Abs(x-y), math.Abs(x+y))/math.Sqrt2 - iconSize*0.09
		return math.Max(d, math.Max(math.Abs(x), math.Abs(y))-iconSize*0.32)
	})
}
