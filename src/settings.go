package main

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/rymdport/portal/filechooser"
)

const (
	modeScan = "Search the roots"
	modeList = "Only the listed repositories"
)

// showSettings opens the settings window on a copy of the current config,
// which Save applies and closing discards.
func (t *tray) showSettings() {
	if t.settings == nil {
		t.settings = t.app.NewWindow("unpushed settings")
		t.settings.SetCloseIntercept(t.settings.Hide)
		t.settings.Resize(fyne.NewSize(640, 720))
	}
	t.mu.Lock()
	d := t.cfg
	for _, paths := range d.paths() {
		*paths = slices.Clone(*paths)
	}
	found := slices.Clone(t.found)
	t.mu.Unlock()
	slices.Sort(found)

	roots := widget.NewMultiLineEntry()
	roots.SetText(strings.Join(mapStrings(d.Roots, tilde), "\n"))
	depth := widget.NewEntry()
	depth.SetText(strconv.Itoa(d.Depth))
	max := widget.NewEntry()
	max.SetText(strconv.Itoa(d.Max))
	terminal := widget.NewEntry()
	terminal.SetText(strings.Join(d.Terminal, " "))
	scanEvery := widget.NewEntry()
	scanEvery.SetText(d.ScanInterval.String())
	remoteEvery := widget.NewEntry()
	remoteEvery.SetText(d.RemoteInterval.String())

	rows := container.NewVBox()
	heading := widget.NewLabel("")
	add := widget.NewButton("Add repository…", nil)
	mode := widget.NewRadioGroup([]string{modeScan, modeList}, nil)
	toggle := func(list *[]string, path string) func(bool) {
		return func(on bool) {
			*list = slices.DeleteFunc(*list, func(p string) bool { return p == path })
			if on {
				*list = append(*list, path)
			}
		}
	}
	var build func()
	build = func() {
		rows.RemoveAll()
		if mode.Selected == modeScan {
			heading.SetText("Repositories found")
			add.Hide()
			for _, path := range found {
				ignore := widget.NewCheck("Ignore", toggle(&d.Ignore, path))
				ignore.SetChecked(slices.Contains(d.Ignore, path))
				remote := widget.NewCheck("Check remote", toggle(&d.CheckRemote, path))
				remote.SetChecked(slices.Contains(d.CheckRemote, path))
				rows.Add(container.NewBorder(nil, nil, nil, container.NewHBox(ignore, remote), widget.NewLabel(tilde(path))))
			}
		} else {
			heading.SetText("Repositories")
			add.Show()
			for _, path := range d.Repos {
				remote := widget.NewCheck("Check remote", toggle(&d.CheckRemote, path))
				remote.SetChecked(slices.Contains(d.CheckRemote, path))
				remove := widget.NewButton("Remove", func() {
					d.Repos = slices.DeleteFunc(d.Repos, func(p string) bool { return p == path })
					build()
				})
				rows.Add(container.NewBorder(nil, nil, nil, container.NewHBox(remote, remove), widget.NewLabel(tilde(path))))
			}
		}
		if len(rows.Objects) == 0 {
			rows.Add(widget.NewLabel("No repositories yet."))
		}
	}
	add.OnTapped = func() {
		home, _ := os.UserHomeDir()
		go func() {
			uris, err := filechooser.OpenFile("", "Add repository", &filechooser.OpenFileOptions{Directory: true, Multiple: true, CurrentFolder: home})
			fyne.Do(func() {
				if err != nil {
					dialog.ShowError(fmt.Errorf("Unable to open the file chooser: %w", err), t.settings)
					return
				}
				for _, uri := range uris {
					path := strings.TrimPrefix(uri, "file://")
					if !isRepo(path) {
						dialog.ShowError(fmt.Errorf("%s is not a git repository", path), t.settings)
					} else if !slices.Contains(d.Repos, path) {
						d.Repos = append(d.Repos, path)
					}
				}
				build()
			})
		}()
	}
	mode.OnChanged = func(m string) {
		// Start a list from what the search shows
		if m == modeList && len(d.Repos) == 0 {
			for _, path := range found {
				if !slices.Contains(d.Ignore, path) {
					d.Repos = append(d.Repos, path)
				}
			}
		}
		roots.Enable()
		depth.Enable()
		if m == modeList {
			roots.Disable()
			depth.Disable()
		}
		build()
	}
	mode.Required = true
	if len(d.Repos) > 0 {
		mode.SetSelected(modeList)
	} else {
		mode.SetSelected(modeScan)
	}

	save := widget.NewButton("Save", func() {
		cfg, err := parseSettings(d, mode.Selected, roots.Text, depth.Text, max.Text, terminal.Text, scanEvery.Text, remoteEvery.Text)
		if err == nil {
			err = t.applyConfig(cfg)
		}
		if err != nil {
			dialog.ShowError(err, t.settings)
			return
		}
		t.settings.Hide()
	})
	save.Importance = widget.HighImportance
	cancel := widget.NewButton("Cancel", t.settings.Hide)

	path, _ := configPath()
	form := widget.NewForm(
		widget.NewFormItem("Repositories", mode),
		widget.NewFormItem("Roots", roots),
		widget.NewFormItem("Depth", depth),
		widget.NewFormItem("Listed in the menu", max),
		widget.NewFormItem("Terminal", terminal),
		widget.NewFormItem("Search every", scanEvery),
		widget.NewFormItem("Check remotes every", remoteEvery),
	)
	t.settings.SetContent(container.NewBorder(
		container.NewVBox(form, container.NewBorder(nil, nil, nil, add, heading)),
		container.NewVBox(
			widget.NewLabelWithStyle(tilde(path), fyne.TextAlignLeading, fyne.TextStyle{Italic: true}),
			container.NewHBox(layout.NewSpacer(), cancel, save),
		),
		nil, nil,
		container.NewVScroll(rows),
	))
	t.settings.Show()
	t.settings.RequestFocus()
}

// parseSettings turns the draft and the window's fields into a config.
func parseSettings(d Config, mode, roots, depth, max, terminal, scanEvery, remoteEvery string) (Config, error) {
	var err error
	if d.Depth, err = strconv.Atoi(strings.TrimSpace(depth)); err != nil {
		return d, fmt.Errorf("Depth must be a number")
	}
	if d.Max, err = strconv.Atoi(strings.TrimSpace(max)); err != nil {
		return d, fmt.Errorf("Listed in the menu must be a number")
	}
	for _, f := range []struct {
		to   *Duration
		text string
	}{{&d.ScanInterval, scanEvery}, {&d.RemoteInterval, remoteEvery}} {
		v, err := time.ParseDuration(strings.TrimSpace(f.text))
		if err != nil || v <= 0 {
			return d, fmt.Errorf("Intervals are durations like 5m or 1h30m")
		}
		f.to.Duration = v
	}
	d.Terminal = strings.Fields(terminal)
	d.Roots = nil
	for line := range strings.Lines(roots) {
		if line = strings.TrimSpace(line); line != "" {
			d.Roots = append(d.Roots, expand(line))
		}
	}
	if mode == modeScan {
		d.Repos = nil
		if len(d.Roots) == 0 {
			return d, fmt.Errorf("Searching needs at least one root")
		}
	} else {
		d.Ignore = nil
		d.CheckRemote = slices.DeleteFunc(d.CheckRemote, func(p string) bool { return !slices.Contains(d.Repos, p) })
		if len(d.Repos) == 0 {
			return d, fmt.Errorf("The list needs at least one repository")
		}
	}
	return d, nil
}

func mapStrings(in []string, f func(string) string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = f(s)
	}
	return out
}
