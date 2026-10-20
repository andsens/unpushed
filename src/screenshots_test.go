//go:build screenshots

package main

// Regenerates the images in docs/ and assets/:
// go test -tags screenshots -run Screenshots .

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
)

func TestScreenshots(t *testing.T) {
	write := func(path string, data []byte) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	render := func(path string, res fyne.Resource, size float32) {
		data, err := renderPNG(res, size)
		if err != nil {
			t.Fatal(err)
		}
		write(path, data)
	}
	test.NewTempApp(t)
	write("../assets/icon.svg", appIcon.Content())
	render("../assets/icon.png", appIcon, 1024)
	for name, look := range map[string]Look{
		"synced": {}, "ahead": {Up: true}, "behind": {Down: true},
		"diverged": {Up: true, Down: true}, "failed": {Up: true, Failed: true},
	} {
		render("../docs/icons/"+name+".png", icon(look), 48)
	}

	// Example paths, shown with ~ for this home
	t.Setenv("HOME", "/home/you")
	cfg := defaultConfig()
	cfg.Roots = []string{"/home/you/src", "/home/you/.dotfiles"}
	cfg.Ignore = []string{"/home/you/src/archive/old-site"}
	cfg.CheckRemote = []string{"/home/you/src/work/api"}
	for variant, th := range map[string]fyne.Theme{"dark": theme.DarkTheme(), "light": theme.LightTheme()} {
		a := test.NewApp()
		a.Settings().SetTheme(th)
		tr := &tray{app: a, cfg: cfg, found: []string{
			"/home/you/.dotfiles", "/home/you/src/archive/old-site", "/home/you/src/homelab",
			"/home/you/src/work/api", "/home/you/src/work/frontend",
		}}
		tr.showSettings()
		f, err := os.Create("../docs/settings-" + variant + ".png")
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, tr.settings.Canvas().Capture()); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
}
