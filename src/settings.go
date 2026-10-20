package main

import (
	"slices"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

// intervals are the interval sliders' stops, roughly logarithmic.
var intervals = []time.Duration{
	time.Minute, 2 * time.Minute, 5 * time.Minute, 10 * time.Minute, 15 * time.Minute,
	30 * time.Minute, time.Hour, 2 * time.Hour, 3 * time.Hour, 6 * time.Hour,
}

// nearestInterval is the index of the stop closest to d.
func nearestInterval(d time.Duration) int {
	best := 0
	for i, stop := range intervals {
		if (stop - d).Abs() < (intervals[best] - d).Abs() {
			best = i
		}
	}
	return best
}

// showSettings opens the settings window on a copy of the current config,
// which Save applies and closing discards.
func (t *tray) showSettings() {
	if t.settings == nil {
		t.settings = t.app.NewWindow("unpushed settings")
		t.settings.SetCloseIntercept(t.settings.Hide)
		t.settings.SetIcon(appIcon)
		t.settings.Resize(fyne.NewSize(640, 760))
	}
	t.mu.Lock()
	d := t.cfg
	for _, paths := range d.paths() {
		*paths = slices.Clone(*paths)
	}
	found := slices.Clone(t.found)
	t.mu.Unlock()

	depth := numberSlider(&d.Depth)
	max := numberSlider(&d.Max)
	scanEvery := intervalSlider(&d.ScanInterval)
	remoteEvery := intervalSlider(&d.RemoteInterval)

	roots := container.NewVBox()
	addRoot := widget.NewButton("Add root…", nil)
	var buildRoots func()
	buildRoots = func() {
		roots.RemoveAll()
		for _, path := range d.Roots {
			remove := widget.NewButton("Remove", func() {
				d.Roots = slices.DeleteFunc(d.Roots, func(p string) bool { return p == path })
				buildRoots()
			})
			roots.Add(container.NewBorder(nil, nil, nil, remove, widget.NewLabel(tilde(path))))
		}
		roots.Add(container.NewHBox(addRoot))
	}
	addRoot.OnTapped = func() {
		t.pickDirs("Add root", func(path string) {
			if !slices.Contains(d.Roots, path) {
				d.Roots = append(d.Roots, path)
			}
		}, buildRoots)
	}
	buildRoots()

	rows := container.NewVBox()
	descending := false
	sortButton := widget.NewButton("", nil)
	toggle := func(list *[]string, path string) func(bool) {
		return func(on bool) {
			*list = slices.DeleteFunc(*list, func(p string) bool { return p == path })
			if on {
				*list = append(*list, path)
			}
		}
	}
	buildRepos := func() {
		rows.RemoveAll()
		sortButton.SetText(map[bool]string{false: "Path ↑", true: "Path ↓"}[descending])
		slices.Sort(found)
		if descending {
			slices.Reverse(found)
		}
		for _, path := range found {
			ignore := widget.NewCheck("Ignore", toggle(&d.Ignore, path))
			ignore.SetChecked(slices.Contains(d.Ignore, path))
			remote := widget.NewCheck("Check remote", toggle(&d.CheckRemote, path))
			remote.SetChecked(slices.Contains(d.CheckRemote, path))
			rows.Add(container.NewBorder(nil, nil, nil, container.NewHBox(ignore, remote), widget.NewLabel(tilde(path))))
		}
		if len(rows.Objects) == 0 {
			rows.Add(widget.NewLabel("No repositories found yet."))
		}
	}
	sortButton.OnTapped = func() {
		descending = !descending
		buildRepos()
	}
	buildRepos()

	form := widget.NewForm(
		widget.NewFormItem("Roots", roots),
		widget.NewFormItem("Depth", depth),
		widget.NewFormItem("Listed in the menu", max),
		widget.NewFormItem("Search every", scanEvery),
		widget.NewFormItem("Check remotes every", remoteEvery),
	)

	save := widget.NewButton("Save", func() {
		if err := t.applyConfig(d); err != nil {
			dialog.ShowError(err, t.settings)
			return
		}
		t.settings.Hide()
	})
	save.Importance = widget.HighImportance
	cancel := widget.NewButton("Cancel", t.settings.Hide)

	t.settings.SetContent(container.NewBorder(
		container.NewVBox(form, container.NewBorder(nil, nil, widget.NewLabel("Repositories found"), sortButton)),
		container.NewHBox(layout.NewSpacer(), cancel, save),
		nil, nil,
		container.NewVScroll(rows),
	))
	t.settings.Show()
	t.settings.RequestFocus()
}

// numberSlider sets *v from 1 to 10.
func numberSlider(v *int) fyne.CanvasObject {
	label := widget.NewLabel(strconv.Itoa(*v))
	s := widget.NewSlider(1, 10)
	s.Value = float64(min(max(*v, 1), 10))
	s.OnChanged = func(f float64) {
		*v = int(f)
		label.SetText(strconv.Itoa(*v))
	}
	return container.NewBorder(nil, nil, nil, label, s)
}

// intervalSlider sets *v to one of the interval stops. Until it's moved, *v
// keeps a value from the config that lies between stops.
func intervalSlider(v *Duration) fyne.CanvasObject {
	label := widget.NewLabel(v.String())
	s := widget.NewSlider(0, float64(len(intervals)-1))
	s.Value = float64(nearestInterval(v.Duration))
	s.OnChanged = func(f float64) {
		v.Duration = intervals[int(f)]
		label.SetText(v.String())
	}
	return container.NewBorder(nil, nil, nil, label, s)
}
