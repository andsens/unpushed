package main

import (
	"bytes"
	"image/png"
	"testing"
)

func TestLook(t *testing.T) {
	for _, c := range []struct {
		look Look
		want string
	}{
		{Look{}, "green"},
		{Look{Up: true}, "yellow"},
		{Look{Down: true}, "yellow"},
		{Look{Up: true, Down: true}, "red"},
		{Look{Failed: true}, "red"},
		{Look{Up: true}.Union(Look{Down: true}), "red"},
	} {
		got := map[any]string{green: "green", yellow: "yellow", red: "red"}[c.look.Color()]
		if got != c.want {
			t.Errorf("%+v is %s, want %s", c.look, got, c.want)
		}
		img, err := png.Decode(bytes.NewReader(draw(c.look.layers(), iconSize)))
		if err != nil {
			t.Fatal(err)
		}
		// The centre is the arrow's white, or the circle's colour without one
		r, g, b, _ := img.At(iconSize/2, iconSize/2).RGBA()
		white := r>>8 == 0xff && g>>8 == 0xff && b>>8 == 0xff
		if arrowAtCentre := (c.look.Up != c.look.Down); white != arrowAtCentre {
			t.Errorf("%+v has white at the centre: %t", c.look, white)
		}
	}
}
