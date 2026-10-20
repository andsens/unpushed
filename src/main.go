package main

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"fyne.io/systray"
	"github.com/fsnotify/fsnotify"
	"github.com/rymdport/portal/filechooser"
)

const (
	scanInterval = 5 * time.Minute
	refreshDelay = 500 * time.Millisecond
	gitTimeout   = 2 * time.Minute
)

type tray struct {
	items   []*systray.MenuItem
	ignores []*systray.MenuItem
	add     *systray.MenuItem
	watcher *fsnotify.Watcher
	refresh *time.Timer
	work    sync.Mutex // serializes git runs
	mu      sync.Mutex // guards the fields below
	cfg     Config
	repos   map[string]*Repo
	shown   []Repo
	failed  map[string]bool
	dirty   map[string]bool // repos changed since their last refresh
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
	t := &tray{cfg: cfg, repos: map[string]*Repo{}, failed: map[string]bool{}, dirty: map[string]bool{}}
	systray.Run(t.ready, nil)
}

func (t *tray) ready() {
	systray.SetIcon(icons[Synced])
	systray.SetTooltip("unpushed")
	for range t.cfg.Max {
		item := systray.AddMenuItem("", "")
		item.Hide()
		t.items = append(t.items, item)
	}
	systray.AddSeparator()
	ignore := systray.AddMenuItem("Ignore", "Stop listing a repository")
	for i, item := range t.items {
		sub := ignore.AddSubMenuItem("", "")
		sub.Hide()
		t.ignores = append(t.ignores, sub)
		go func() {
			for {
				select {
				case <-item.ClickedCh:
					t.click(i)
				case <-sub.ClickedCh:
					t.ignore(i)
				}
			}
		}()
	}
	t.add = systray.AddMenuItem("Add repository…", "Add a repository to repos")
	if len(t.cfg.Repos) == 0 {
		t.add.Hide()
	}
	go func() {
		for range t.add.ClickedCh {
			t.addRepo()
		}
	}()

	w, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Error(fmt.Sprintf("Unable to watch repositories: %s", err))
		systray.Quit()
		return
	}
	t.watcher = w
	// Debounced, a commit or fetch touches several files
	t.refresh = time.AfterFunc(time.Hour, t.refreshDirty)
	t.refresh.Stop()
	go func() {
		t.scan()
		scan := time.Tick(scanInterval)
		for {
			select {
			case <-scan:
				t.scan()
			case ev := <-w.Events:
				if ev.Has(fsnotify.Create) && strings.Contains(ev.Name, "/.git/refs/") {
					if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() {
						_ = w.Add(ev.Name)
					}
				}
				if repo, _, ok := strings.Cut(ev.Name, "/.git/"); ok && relevant(ev) {
					t.mu.Lock()
					t.dirty[repo] = true
					t.mu.Unlock()
					t.refresh.Reset(refreshDelay)
				}
			case err := <-w.Errors:
				slog.Warn(fmt.Sprintf("Watcher error: %s", err))
			}
		}
	}()
}

// scan rediscovers repositories, queries the remotes of those in checkRemote
// and refreshes what changed.
func (t *tray) scan() {
	t.mu.Lock()
	cfg := t.cfg
	t.mu.Unlock()
	var found []Repo
	for _, path := range cfg.Repos {
		found = append(found, Repo{Path: path, Name: filepath.Base(path)})
	}
	for _, root := range cfg.Roots {
		if len(cfg.Repos) > 0 {
			break
		}
		ctx, cancel := context.WithTimeout(context.Background(), scanInterval)
		repos, err := Scan(ctx, root, cfg.Depth)
		cancel()
		if err != nil {
			slog.Error(err.Error())
		}
		for _, r := range repos {
			if !slices.Contains(cfg.Ignore, r.Path) {
				found = append(found, r)
			}
		}
	}
	watch(t.watcher, found)
	t.mu.Lock()
	repos := map[string]*Repo{}
	for _, r := range found {
		if known, ok := t.repos[r.Path]; ok {
			repos[r.Path] = known
		} else {
			repos[r.Path] = &r
			t.dirty[r.Path] = true
		}
	}
	t.repos = repos
	t.mu.Unlock()
	for _, path := range cfg.CheckRemote {
		if _, ok := repos[path]; !ok {
			continue
		}
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

// render updates the menu and the tray LED. Called with mu held.
func (t *tray) render() {
	var all []Repo
	for r := range maps.Values(t.repos) {
		all = append(all, *r)
	}
	tray := Synced
	for _, r := range all {
		ahead, behind, unknown := r.Counts()
		if ahead > 0 {
			tray = Ahead
		} else if (behind > 0 || unknown) && tray == Synced {
			tray = Behind
		}
	}
	systray.SetIcon(icons[tray])

	t.shown = MostActive(all, t.cfg.Max)
	for i, item := range t.items {
		if i >= len(t.shown) {
			item.Hide()
			t.ignores[i].Hide()
			continue
		}
		r := t.shown[i]
		state := t.state(r)
		name := strings.ReplaceAll(r.Name, "_", "__") // _ marks a mnemonic in dbusmenu
		title := name
		ahead, behind, unknown := r.Counts()
		if ahead > 0 {
			title += fmt.Sprintf("  ↑%d", ahead)
		}
		if unknown {
			title += "  ↓?"
		} else if behind > 0 {
			title += fmt.Sprintf("  ↓%d", behind)
		}
		if state == Failed {
			title += "  (open shell)"
		}
		var tip []string
		for _, b := range r.Branches {
			if b.Unknown {
				tip = append(tip, fmt.Sprintf("%s ↑%d, remote has unfetched commits", b.Name, b.Ahead))
			} else if b.Ahead > 0 || b.Behind > 0 || b.Gone {
				tip = append(tip, fmt.Sprintf("%s ↑%d ↓%d%s", b.Name, b.Ahead, b.Behind, map[bool]string{true: " (upstream gone)"}[b.Gone]))
			}
		}
		item.SetTitle(title)
		item.SetTooltip(strings.Join(tip, "\n"))
		item.SetIcon(icons[state])
		item.Show()
		t.ignores[i].SetTitle(name)
		t.ignores[i].Show()
	}
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
	terminal := t.cfg.Terminal
	t.mu.Unlock()

	switch state {
	case Synced:
		return
	case Failed:
		cmd := exec.Command(terminal[0], append(terminal[1:], r.Path)...)
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

func (t *tray) ignore(i int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if i >= len(t.shown) {
		return
	}
	path := t.shown[i].Path
	if err := ignoreRepo(path); err != nil {
		slog.Error(err.Error())
		return
	}
	if err := t.reload(); err != nil {
		slog.Error(err.Error())
		return
	}
	delete(t.repos, path)
	t.render()
}

func (t *tray) addRepo() {
	home, _ := os.UserHomeDir()
	uris, err := filechooser.OpenFile("", "Add repository", &filechooser.OpenFileOptions{Directory: true, CurrentFolder: home})
	if err != nil {
		slog.Error(fmt.Sprintf("Unable to open file chooser: %s", err))
		return
	}
	for _, uri := range uris {
		if err := addRepo(strings.TrimPrefix(uri, "file://")); err != nil {
			slog.Error(err.Error())
		}
	}
	t.mu.Lock()
	err = t.reload()
	t.mu.Unlock()
	if err != nil {
		slog.Error(err.Error())
		return
	}
	t.scan()
}

// reload rereads the config after an edit. Max stays, the menu is built.
// Called with mu held.
func (t *tray) reload() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	cfg.Max = t.cfg.Max
	t.cfg = cfg
	return nil
}
