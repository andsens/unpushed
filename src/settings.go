package main

import (
	"image/color"
	"slices"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
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
		t.settings.Resize(fyne.NewSize(680, 860))
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
		if len(roots.Objects) == 0 {
			roots.Add(widget.NewLabel("No roots, nothing is searched."))
		}
	}
	addRoot.OnTapped = func() {
		t.pickDirs("Add root", func(path string) {
			if !slices.Contains(d.Roots, path) {
				d.Roots = append(d.Roots, path)
			}
		}, buildRoots)
	}
	buildRoots()

	reposTable := t.reposTable(found, &d)

	rootsList := container.NewVScroll(roots)
	rootsList.SetMinSize(fyne.NewSize(0, 110))
	search := group("Scanning", container.NewVBox(
		container.NewBorder(nil, nil, widget.NewLabel("Scanning roots and repos"), addRoot),
		sunken(rootsList),
		widget.NewForm(
			widget.NewFormItem("Scanning directory depth", depth),
			widget.NewFormItem("Scanning interval", scanEvery),
		),
	))
	menu := group("Menu", widget.NewForm(
		widget.NewFormItem("Max repos in the menu", max),
		widget.NewFormItem("Remote check interval", remoteEvery),
	))
	// The sunken table is frame enough
	repos := container.NewBorder(
		widget.NewLabelWithStyle("Repositories found", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), nil, nil, nil,
		sunken(reposTable),
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

	t.settings.SetContent(container.NewPadded(container.NewBorder(
		container.NewVBox(search, menu),
		container.NewHBox(layout.NewSpacer(), cancel, save),
		nil, nil,
		repos,
	)))
	t.settings.Show()
	t.settings.RequestFocus()
}

// group frames content with a border under a bold title.
func group(title string, content fyne.CanvasObject) fyne.CanvasObject {
	frame := canvas.NewRectangle(color.Transparent)
	frame.StrokeColor = theme.Color(theme.ColorNameSeparator)
	frame.StrokeWidth = 1
	frame.CornerRadius = theme.InputRadiusSize()
	return container.NewBorder(
		widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), nil, nil, nil,
		container.NewStack(frame, container.NewPadded(content)),
	)
}

// reposTable lists the found repositories with a toggle each for ignoring
// them and checking their remote. Clicking a header sorts by that column.
func (t *tray) reposTable(found []string, d *Config) fyne.CanvasObject {
	headers := []string{"Path", "Include", "Refresh"}
	icons := []fyne.Resource{nil, theme.VisibilityIcon(), theme.ViewRefreshIcon()}
	// Whether a column's toggle is on for path; the eye is lit while the repo
	// is included, the refresh icon while its remote is checked
	on := func(col int, path string) bool {
		if col == 1 {
			return !slices.Contains(d.Ignore, path)
		}
		return slices.Contains(d.CheckRemote, path)
	}
	flip := func(col int, path string) {
		list := &d.CheckRemote
		if col == 1 {
			list = &d.Ignore
		}
		if slices.Contains(*list, path) {
			*list = slices.DeleteFunc(*list, func(p string) bool { return p == path })
		} else {
			*list = append(*list, path)
		}
	}
	sortCol, descending := 0, false
	sortRows := func() {
		slices.SortFunc(found, func(a, b string) int {
			c := 0
			if sortCol > 0 && on(sortCol, a) != on(sortCol, b) {
				c = map[bool]int{true: -1, false: 1}[on(sortCol, a)]
			}
			if c == 0 {
				c = strings.Compare(a, b)
			}
			if descending {
				return -c
			}
			return c
		})
	}
	sortRows()

	var table *widget.Table
	table = widget.NewTableWithHeaders(
		func() (int, int) { return len(found), len(headers) },
		func() fyne.CanvasObject {
			b := widget.NewButtonWithIcon("", nil, nil)
			b.Importance = widget.LowImportance
			l := widget.NewLabel("")
			l.Truncation = fyne.TextTruncateEllipsis
			return container.NewStack(l, b)
		},
		func(id widget.TableCellID, o fyne.CanvasObject) {
			l, b := o.(*fyne.Container).Objects[0].(*widget.Label), o.(*fyne.Container).Objects[1].(*widget.Button)
			path := found[id.Row]
			if id.Col == 0 {
				l.SetText(tilde(path))
				l.Show()
				b.Hide()
				return
			}
			l.Hide()
			b.Show()
			if on(id.Col, path) {
				b.SetIcon(icons[id.Col])
			} else {
				b.SetIcon(theme.NewDisabledResource(icons[id.Col]))
			}
			b.OnTapped = func() {
				flip(id.Col, path)
				table.RefreshItem(id)
			}
		},
	)
	table.ShowHeaderColumn = false
	table.CreateHeader = func() fyne.CanvasObject {
		b := widget.NewButton("", nil)
		b.Importance = widget.LowImportance
		b.Alignment = widget.ButtonAlignLeading
		return b
	}
	table.UpdateHeader = func(id widget.TableCellID, o fyne.CanvasObject) {
		b := o.(*widget.Button)
		text := headers[id.Col]
		if id.Col == sortCol {
			text += map[bool]string{false: " ↑", true: " ↓"}[descending]
		}
		b.SetText(text)
		b.OnTapped = func() {
			descending = id.Col == sortCol && !descending
			sortCol = id.Col
			sortRows()
			table.Refresh()
		}
	}
	// The path takes what the toggle columns leave
	const toggleWidth = 100
	table.SetColumnWidth(1, toggleWidth)
	table.SetColumnWidth(2, toggleWidth)
	return container.New(&stretchFirst{table, 2 * toggleWidth}, table)
}

// stretchFirst sizes a table's first column to the width its other columns
// leave, which Table doesn't do on its own.
type stretchFirst struct {
	table *widget.Table
	rest  float32
}

func (s *stretchFirst) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	// Less the separators between the columns and the scroller's padding
	s.table.SetColumnWidth(0, max(size.Width-s.rest-2*theme.SeparatorThicknessSize()-2*theme.Padding(), 100))
	objects[0].Resize(size)
}

func (s *stretchFirst) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return objects[0].MinSize()
}

// sunken puts content on the input background with a border, the way lists
// sit in the window.
func sunken(content fyne.CanvasObject) fyne.CanvasObject {
	bg := canvas.NewRectangle(theme.Color(theme.ColorNameInputBackground))
	bg.StrokeColor = theme.Color(theme.ColorNameInputBorder)
	bg.StrokeWidth = 1
	bg.CornerRadius = theme.InputRadiusSize()
	return container.NewStack(bg, container.NewPadded(content))
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
