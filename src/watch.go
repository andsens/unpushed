package main

import (
	"errors"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

// watchDirs lists the directories in a repo's .git whose changes can move a
// ref or the reflog of HEAD. objects/ and the rest of the tree are left out.
func watchDirs(repo string) []string {
	gitDir := filepath.Join(repo, ".git")
	dirs := []string{gitDir, filepath.Join(gitDir, "logs"), filepath.Join(gitDir, "reftable")}
	// Loose refs nest by name (refs/remotes/origin/feature/x)
	_ = filepath.WalkDir(filepath.Join(gitDir, "refs"), func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			dirs = append(dirs, path)
		}
		return nil
	})
	return dirs
}

// relevant drops events for files that change without moving a ref.
func relevant(ev fsnotify.Event) bool {
	name := filepath.Base(ev.Name)
	return !strings.HasSuffix(name, ".lock") && name != "index" && !ev.Has(fsnotify.Chmod)
}

// watch replaces the watched set with the directories of repos.
func watch(w *fsnotify.Watcher, repos []Repo) {
	want := map[string]bool{}
	for _, r := range repos {
		for _, dir := range watchDirs(r.Path) {
			want[dir] = true
		}
	}
	for _, dir := range w.WatchList() {
		if !want[dir] {
			_ = w.Remove(dir)
		}
	}
	for dir := range want {
		// reftable/ and logs/ may not exist, a new loose ref dir is picked up by
		// the Create event on its parent
		if err := w.Add(dir); err != nil && !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("Unable to watch", "dir", dir, "err", err)
		}
	}
}
