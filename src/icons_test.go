package main

import (
	"bytes"
	"fmt"
	"image/png"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/software"
	"fyne.io/fyne/v2/test"
)

// renderPNG draws an SVG resource as a size x size PNG.
func renderPNG(res fyne.Resource, size float32) ([]byte, error) {
	c := software.NewTransparentCanvas()
	c.SetPadded(false)
	c.SetContent(canvas.NewImageFromResource(res))
	c.Resize(fyne.NewSquareSize(size))
	var buf bytes.Buffer
	if err := png.Encode(&buf, c.Capture()); err != nil {
		return nil, fmt.Errorf("Unable to encode %s: %w", res.Name(), err)
	}
	return buf.Bytes(), nil
}

func TestWorst(t *testing.T) {
	order := []Look{{}, {Down: true}, {Up: true}, {Up: true, Down: true}, {Failed: true}}
	for i, l := range order {
		for j, o := range order {
			if got := l.Worst(o); got != order[max(i, j)] {
				t.Errorf("%+v worst of %+v is %+v", l, o, got)
			}
		}
	}
}

func TestLook(t *testing.T) {
	test.NewTempApp(t)
	for _, c := range []struct {
		look Look
		want string
	}{
		{Look{}, "green"},
		{Look{Up: true}, "yellow"},
		{Look{Down: true}, "yellow"},
		{Look{Up: true, Down: true}, "red"},
		{Look{Failed: true}, "red"},
		// One repo ahead and another behind isn't a divergence
		{Look{Down: true}.Worst(Look{Up: true}), "yellow"},
		{Look{Up: true}.Worst(Look{Failed: true}), "red"},
	} {
		got := map[any]string{green: "green", yellow: "yellow", red: "red"}[c.look.Color()]
		if got != c.want {
			t.Errorf("%+v is %s, want %s", c.look, got, c.want)
		}
		data, err := renderPNG(icon(c.look), 64)
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		// The centre is white under a single arrow or the cross, else the
		// circle's colour
		r, g, b, _ := img.At(32, 32).RGBA()
		white := r>>8 > 0xf0 && g>>8 > 0xf0 && b>>8 > 0xf0
		if markAtCentre := c.look.Failed || c.look.Up != c.look.Down; white != markAtCentre {
			t.Errorf("%+v has white at the centre: %t", c.look, white)
		}
	}
}
