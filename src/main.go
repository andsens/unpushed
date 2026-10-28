package main

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/driver/desktop"
	"github.com/docopt/docopt-go"
	"github.com/fsnotify/fsnotify"
)

const usage = `unpushed - Tray icon for unpushed and unpulled commits
Usage:
  unpushed [--ro-config PATH] [--config PATH]

Options:
  --ro-config PATH  Base config, read-only. The user's config applies on top.
  --config PATH     The user's config, which the settings window writes
                    [default: the config directory's unpushed.json]
`

const (
	refreshDelay = 500 * time.Millisecond
	gitTimeout   = 2 * time.Minute
)

type tray struct {
	app      fyne.App
	desk     desktop.App
	settings fyne.Window
	watcher  *fsnotify.Watcher
	refresh  *time.Timer
	reconfig chan struct{} // the config changed, rescan
	work     sync.Mutex    // serializes git runs
	base     string        // the base config's path
	user     string        // the user's config's path
	mu       sync.Mutex    // guards the fields below
	cfg      Config
	lower    Config   // cfg without the user's config
	found    []string // what the last scan found, ignored repos included
	repos    map[string]*Repo
	shown    []Repo
	failed   map[string]bool
	dirty    map[string]bool // repos changed since their last refresh
}

func main() {
	var params struct {
		ROConfig string `docopt:"--ro-config"`
		Config   string `docopt:"--config"`
	}
	opts, err := docopt.ParseDoc(usage)
	if err == nil {
		err = opts.Bind(&params)
	}
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
	if params.Config == "the config directory's unpushed.json" {
		params.Config, err = userConfigPath()
	}
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
	cfg, lower, err := loadConfig(params.ROConfig, params.Config)
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Error(fmt.Sprintf("Unable to watch repositories: %s", err))
		os.Exit(1)
	}
	a := app.NewWithID("io.github.andsens.unpushed")
	desk, ok := a.(desktop.App)
	if !ok {
		slog.Error("No system tray on this platform")
		os.Exit(1)
	}
	t := &tray{
		app: a, desk: desk, watcher: w, reconfig: make(chan struct{}, 1), base: params.ROConfig, user: params.Config, cfg: cfg, lower: lower,
		repos: map[string]*Repo{}, failed: map[string]bool{}, dirty: map[string]bool{},
	}
	// Set before Run, render's updates go through fyne.Do once it runs
	desk.SetSystemTrayIcon(icon(Look{}))
	desk.SetSystemTrayMenu(fyne.NewMenu("unpushed", fyne.NewMenuItem("Settings…", t.showSettings)))
	// Debounced, a commit or fetch touches several files
	t.refresh = time.AfterFunc(time.Hour, t.refreshDirty)
	t.refresh.Stop()
	go t.loop()
	a.Run()
}

func (t *tray) loop() {
	t.scan()
	t.lsRemotes()
	// Read on every reset, the config may have changed
	intervals := func() (scan, remote time.Duration) {
		t.mu.Lock()
		defer t.mu.Unlock()
		return t.cfg.ScanInterval.Duration, t.cfg.RemoteInterval.Duration
	}
	scanEvery, remoteEvery := intervals()
	scan, remote := time.NewTimer(scanEvery), time.NewTimer(remoteEvery)
	for {
		// Each timer rearms only itself, a shorter remote interval would
		// otherwise keep pushing the scan back
		select {
		case <-scan.C:
			t.scan()
			scanEvery, _ = intervals()
			scan.Reset(scanEvery)
		case <-remote.C:
			t.lsRemotes()
			_, remoteEvery = intervals()
			remote.Reset(remoteEvery)
		case <-t.reconfig:
			t.scan()
			t.lsRemotes()
			scanEvery, remoteEvery = intervals()
			scan.Reset(scanEvery)
			remote.Reset(remoteEvery)
		case ev := <-t.watcher.Events:
			sep := string(filepath.Separator)
			if ev.Has(fsnotify.Create) && strings.Contains(ev.Name, sep+".git"+sep+"refs"+sep) {
				if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() {
					_ = t.watcher.Add(ev.Name)
				}
			}
			if repo, _, ok := strings.Cut(ev.Name, sep+".git"+sep); ok && relevant(ev) {
				t.mu.Lock()
				t.dirty[repo] = true
				t.mu.Unlock()
				t.refresh.Reset(refreshDelay)
			}
		case err := <-t.watcher.Errors:
			slog.Warn(fmt.Sprintf("Watcher error: %s", err))
		}
	}
}

// scan rediscovers repositories and refreshes the new ones.
func (t *tray) scan() {
	t.mu.Lock()
	cfg := t.cfg
	t.mu.Unlock()
	roots, pruned := cfg.searched()
	found := FindRepos(roots, pruned, cfg.Depth, cfg.ScanInterval.Duration)
	var paths []string
	var listed []Repo
	for _, r := range found {
		paths = append(paths, r.Path)
		if !cfg.Repos[r.Path].Ignore {
			listed = append(listed, r)
		}
	}
	watch(t.watcher, listed)
	t.mu.Lock()
	t.found = paths
	repos := map[string]*Repo{}
	for _, r := range listed {
		if known, ok := t.repos[r.Path]; ok {
			repos[r.Path] = known
		} else {
			repos[r.Path] = &r
			t.dirty[r.Path] = true
		}
	}
	t.repos = repos
	t.mu.Unlock()
	t.refreshDirty()
}

// lsRemotes queries the remotes of the repos set to checkRemote.
func (t *tray) lsRemotes() {
	t.mu.Lock()
	var paths []string
	for path, r := range t.cfg.Repos {
		if _, ok := t.repos[path]; ok && r.CheckRemote {
			paths = append(paths, path)
		}
	}
	// Repos taken out of checkRemote stop counting what the remote has
	for path, r := range t.repos {
		if r.RemoteHeads != nil && !t.cfg.Repos[path].CheckRemote {
			r.RemoteHeads = nil
			t.dirty[path] = true
		}
	}
	t.mu.Unlock()
	for _, path := range paths {
		r := Repo{Path: path}
		ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
		err := r.LsRemote(ctx)
		cancel()
		if err != nil {
			slog.Warn(err.Error())
			continue
		}
		t.mu.Lock()
		if known, ok := t.repos[path]; ok {
			known.RemoteHeads = r.RemoteHeads
			t.dirty[path] = true
		}
		t.mu.Unlock()
	}
	t.refreshDirty()
}

func (t *tray) refreshDirty() {
	t.mu.Lock()
	var dirty []Repo
	for path := range t.dirty {
		if r, ok := t.repos[path]; ok {
			dirty = append(dirty, *r)
		}
	}
	clear(t.dirty)
	t.mu.Unlock()

	t.work.Lock()
	for i := range dirty {
		ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
		if err := dirty[i].Refresh(ctx); err != nil {
			slog.Error(err.Error())
		}
		cancel()
	}
	t.work.Unlock()

	t.mu.Lock()
	defer t.mu.Unlock()
	for _, r := range dirty {
		if known, ok := t.repos[r.Path]; ok {
			// ls-remote may have updated it meanwhile, that marked it dirty again
			r.RemoteHeads = known.RemoteHeads
			t.repos[r.Path] = &r
		}
	}
	t.render()
}

// render rebuilds the tray menu and icon. Called with mu held.
func (t *tray) render() {
	var all []Repo
	tray := Look{}
	for r := range maps.Values(t.repos) {
		if r.HasRemote {
			all = append(all, *r)
			tray = tray.Worst(t.look(*r))
		}
	}
	t.shown = MenuRepos(all, t.cfg.Max, func(path string) bool { return t.cfg.Repos[path].Pinned },
		func(r Repo) bool { return t.look(r) != Look{} })
	var items []*fyne.MenuItem
	for i, r := range t.shown {
		title := r.Name
		ahead, behind, unknown := r.Counts()
		if ahead > 0 {
			title += fmt.Sprintf("  ↑%d", ahead)
		}
		if unknown {
			title += "  ↓?"
		} else if behind > 0 {
			title += fmt.Sprintf("  ↓%d", behind)
		}
		if t.state(r) == Failed {
			title += "  (open shell)"
		}
		items = append(items, fyne.NewMenuItemWithIcon(title, icon(t.look(r)), func() { go t.click(i) }))
	}
	if len(items) == 0 {
		items = append(items, &fyne.MenuItem{Label: "No repositories", Disabled: true})
	}
	// fyne adds Quit
	items = append(items, fyne.NewMenuItemSeparator(), fyne.NewMenuItem("Settings…", t.showSettings))
	menu := fyne.NewMenu("unpushed", items...)
	fyne.Do(func() {
		t.desk.SetSystemTrayIcon(icon(tray))
		t.desk.SetSystemTrayMenu(menu)
	})
}

// look adds a failed sync to the repo's look. Called with mu held.
func (t *tray) look(r Repo) Look {
	l := r.Look()
	l.Failed = l.Failed || t.failed[r.Path]
	return l
}

// state is the repo's own state, unless its last sync failed. Called with mu held.
func (t *tray) state(r Repo) State {
	state := r.State()
	if state == Synced {
		delete(t.failed, r.Path)
	} else if t.failed[r.Path] {
		state = Failed
	}
	return state
}

func (t *tray) click(i int) {
	t.mu.Lock()
	if i >= len(t.shown) {
		t.mu.Unlock()
		return
	}
	r := t.shown[i]
	state := t.state(r)
	delete(t.failed, r.Path)
	t.mu.Unlock()

	switch state {
	case Synced:
		return
	case Failed:
		cmd := terminal(r.Path)
		cmd.Dir = r.Path
		if err := cmd.Start(); err != nil {
			slog.Error(fmt.Sprintf("Unable to start terminal: %s", err))
		} else {
			go cmd.Wait()
		}
	default:
		t.work.Lock()
		ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
		err := r.Sync(ctx)
		cancel()
		t.work.Unlock()
		if err != nil {
			slog.Error(err.Error())
			t.mu.Lock()
			t.failed[r.Path] = true
			t.mu.Unlock()
		}
	}
	t.mu.Lock()
	t.dirty[r.Path] = true
	t.mu.Unlock()
	t.refreshDirty()
}

// applyConfig saves cfg and rescans with it.
func (t *tray) applyConfig(cfg Config) error {
	t.mu.Lock()
	lower := t.lower
	t.mu.Unlock()
	if err := saveConfig(t.user, cfg, lower); err != nil {
		return err
	}
	loaded, lower, err := loadConfig(t.base, t.user)
	if err != nil {
		return err
	}
	t.mu.Lock()
	t.cfg, t.lower = loaded, lower
	t.mu.Unlock()
	select {
	case t.reconfig <- struct{}{}:
	default:
	}
	return nil
}
