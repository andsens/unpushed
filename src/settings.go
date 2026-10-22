package main

import (
	"image/color"
	"maps"
	"path/filepath"
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
		t.settings.Resize(fyne.NewSize(680, 960))
	}
	t.mu.Lock()
	d := t.cfg
	d.Roots, d.Repos = maps.Clone(d.Roots), maps.Clone(d.Repos)
	found := slices.Clone(t.found)
	// What the menu shows for each repo it can list
	looks := map[string]Look{}
	for path, r := range t.repos {
		if r.HasRemote {
			looks[path] = t.look(*r)
		}
	}
	t.mu.Unlock()

	depth := numberSlider(&d.Depth, 10)
	max := numberSlider(&d.Max, 30)
	scanEvery := intervalSlider(&d.ScanInterval)
	remoteEvery := intervalSlider(&d.RemoteInterval)

	// Each table lists the roots with include at its value. The trash removes
	// a root whichever config set it, saveConfig writes null for the base's.
	var refreshRoots []func()
	roots := func(include bool) (fyne.CanvasObject, fyne.CanvasObject) {
		table, refresh := pathTable(func() []string {
			var paths []string
			for path, r := range d.Roots {
				if r.Include == include {
					paths = append(paths, path)
				}
			}
			return paths
		}, []column{toggle("Remove", theme.DeleteIcon(), func(string) bool { return true }, func(p string) { delete(d.Roots, p) })})
		refreshRoots = append(refreshRoots, refresh)
		title := map[bool]string{true: "Add root…", false: "Add exclusion…"}[include]
		add := widget.NewButton(title, func() {
			t.pickDirs(strings.TrimSuffix(title, "…"), func(path string) { d.Roots[filepath.Clean(path)] = Root{Include: include} }, func() {
				for _, refresh := range refreshRoots {
					refresh()
				}
			})
		})
		// Room for a few roots, the repos table takes the rest
		height := canvas.NewRectangle(color.Transparent)
		height.SetMinSize(fyne.NewSize(0, 150))
		return add, sunken(container.NewStack(height, table))
	}
	addRoot, includedTable := roots(true)
	addExclusion, excludedTable := roots(false)

	// The eye is lit while the repo is included, the others while their
	// setting is on
	repo := func(p string, set func(*RepoConfig)) {
		r := d.Repos[p]
		set(&r)
		d.Repos[p] = r
	}
	reposTable, _ := pathTable(func() []string { return found }, []column{
		{"Status", func(p string) fyne.Resource {
			if l, ok := looks[p]; ok {
				return icon(l)
			}
			return nil
		}, func(p string) int {
			// Red, yellow, green, then the ones without a status
			if l, ok := looks[p]; ok {
				return map[color.NRGBA]int{red: 0, yellow: 1, green: 2}[l.Color()]
			}
			return 3
		}, nil},
		toggle("Include", theme.VisibilityIcon(), func(p string) bool { return !d.Repos[p].Ignore }, func(p string) {
			repo(p, func(r *RepoConfig) { r.Ignore = !r.Ignore })
		}),
		toggle("Refresh", theme.ViewRefreshIcon(), func(p string) bool { return d.Repos[p].CheckRemote }, func(p string) {
			repo(p, func(r *RepoConfig) { r.CheckRemote = !r.CheckRemote })
		}),
		toggle("Pin", pinIcon, func(p string) bool { return d.Repos[p].Pinned }, func(p string) {
			repo(p, func(r *RepoConfig) { r.Pinned = !r.Pinned })
		}),
	})

	search := group("Scanning", container.NewVBox(
		container.NewBorder(nil, nil, widget.NewLabel("Searched directories"), addRoot),
		includedTable,
		container.NewBorder(nil, nil, widget.NewLabel("Excluded directories"), addExclusion),
		excludedTable,
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

// column is an icon per path in a pathTable, sorted by rank. A nil tap
// makes it read-only.
type column struct {
	header string
	icon   func(path string) fyne.Resource
	rank   func(path string) int
	tap    func(path string)
}

// toggle is a column whose icon is lit while on, listing those first.
func toggle(header string, res fyne.Resource, on func(path string) bool, flip func(path string)) column {
	return column{header, func(p string) fyne.Resource {
		if on(p) {
			return res
		}
		return theme.NewDisabledResource(res)
	}, func(p string) int {
		if on(p) {
			return 0
		}
		return 1
	}, flip}
}

// pathTable lists paths with an icon per column, and refresh rereads them.
// Clicking a header sorts by that column.
func pathTable(rows func() []string, cols []column) (fyne.CanvasObject, func()) {
	headers := []string{"Path"}
	for _, c := range cols {
		headers = append(headers, c.header)
	}
	paths := rows()
	sortCol, descending := 0, false
	sortRows := func() {
		slices.SortFunc(paths, func(a, b string) int {
			c := 0
			if sortCol > 0 {
				rank := cols[sortCol-1].rank
				c = rank(a) - rank(b)
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
	refresh := func() {
		paths = rows()
		sortRows()
		table.Refresh()
	}
	table = widget.NewTableWithHeaders(
		func() (int, int) { return len(paths), len(headers) },
		func() fyne.CanvasObject {
			b := widget.NewButtonWithIcon("", nil, nil)
			b.Importance = widget.LowImportance
			l := widget.NewLabel("")
			l.Truncation = fyne.TextTruncateEllipsis
			return container.NewStack(l, b)
		},
		func(id widget.TableCellID, o fyne.CanvasObject) {
			l, b := o.(*fyne.Container).Objects[0].(*widget.Label), o.(*fyne.Container).Objects[1].(*widget.Button)
			path := paths[id.Row]
			if id.Col == 0 {
				l.SetText(tilde(path))
				l.Show()
				b.Hide()
				return
			}
			l.Hide()
			b.Show()
			c := cols[id.Col-1]
			b.SetIcon(c.icon(path))
			b.OnTapped = nil
			if c.tap == nil {
				return
			}
			b.OnTapped = func() {
				c.tap(path)
				// Rows stay put under the pointer, unless one went
				if len(rows()) != len(paths) {
					refresh()
				} else {
					table.RefreshItem(id)
				}
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
	const toggleWidth = 90
	for i := range cols {
		table.SetColumnWidth(i+1, toggleWidth)
	}
	return container.New(&stretchFirst{table, len(cols), toggleWidth}, table), refresh
}

// stretchFirst sizes a table's first column to the width its other columns
// leave, which Table doesn't do on its own.
type stretchFirst struct {
	table *widget.Table
	cols  int     // the other columns
	width float32 // of each other column
}

func (s *stretchFirst) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	// Table spaces its columns by the padding
	rest := float32(s.cols) * (s.width + theme.Padding())
	s.table.SetColumnWidth(0, max(size.Width-rest, 100))
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

// numberSlider sets *v from 1 to top.
func numberSlider(v *int, top int) fyne.CanvasObject {
	label := widget.NewLabel(strconv.Itoa(*v))
	s := widget.NewSlider(1, float64(top))
	s.Value = float64(min(max(*v, 1), top))
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
