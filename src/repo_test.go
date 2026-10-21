package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s: %s", args, err, out)
	}
}

// setup returns two clones (a, b) of a bare remote with one commit on main.
func setup(t *testing.T) (root, a, b string) {
	root = t.TempDir()
	// An empty global config, /dev/null doesn't exist on Windows
	global := filepath.Join(root, "gitconfig")
	if err := os.WriteFile(global, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	run(t, root, "git", "init", "--quiet", "--bare", "--initial-branch=main", "remote.git")
	run(t, root, "git", "clone", "--quiet", "remote.git", "a")
	a, b = filepath.Join(root, "a"), filepath.Join(root, "b")
	run(t, a, "git", "commit", "--quiet", "--allow-empty", "-m", "init")
	run(t, a, "git", "push", "--quiet", "origin", "main")
	run(t, root, "git", "clone", "--quiet", "remote.git", "b")
	return
}

func state(t *testing.T, path string) Repo {
	t.Helper()
	r := Repo{Path: path}
	if err := r.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	return r
}

func expect(t *testing.T, path string, want State) {
	t.Helper()
	if got := state(t, path).State(); got != want {
		t.Fatalf("%s: state %d, want %d", path, got, want)
	}
}

func TestSyncPushAndPull(t *testing.T) {
	_, a, b := setup(t)
	expect(t, a, Synced)
	run(t, a, "git", "commit", "--quiet", "--allow-empty", "-m", "two")
	expect(t, a, Ahead)
	r := state(t, a)
	if err := r.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	expect(t, a, Synced)

	run(t, b, "git", "fetch", "--quiet")
	expect(t, b, Behind)
	r = state(t, b)
	if err := r.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	expect(t, b, Synced)
}

func TestNoUpstreamPushesToSoleRemote(t *testing.T) {
	_, a, _ := setup(t)
	run(t, a, "git", "switch", "--quiet", "-c", "feature")
	expect(t, a, Synced) // Nothing a remote doesn't have
	run(t, a, "git", "commit", "--quiet", "--allow-empty", "-m", "feature")
	expect(t, a, Ahead)
	r := state(t, a)
	if err := r.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	expect(t, a, Synced)
}

func TestDivergedFails(t *testing.T) {
	_, a, b := setup(t)
	run(t, a, "git", "commit", "--quiet", "--allow-empty", "-m", "a")
	run(t, a, "git", "push", "--quiet")
	run(t, b, "git", "commit", "--quiet", "--allow-empty", "-m", "b")
	expect(t, b, Ahead) // Not fetched yet
	r := state(t, b)
	if err := r.Sync(context.Background()); err == nil {
		t.Fatal("Sync of a diverged branch succeeded")
	}
	expect(t, b, Failed)
}

func TestScanAndOrder(t *testing.T) {
	root, a, b := setup(t)
	deep := filepath.Join(root, "x", "y", "z")
	hidden := filepath.Join(root, ".hidden", "c")
	// A glob unless escaped, it would match w1
	glob := filepath.Join(root, "w[1]")
	for _, dir := range []string{deep, hidden, glob} {
		run(t, root, "git", "init", "--quiet", dir)
	}
	old := time.Now().Add(-time.Hour)
	for _, r := range []string{a, deep} {
		_ = os.Chtimes(filepath.Join(r, ".git", "logs", "HEAD"), old, old)
		_ = os.Chtimes(filepath.Join(r, ".git", "HEAD"), old, old)
	}
	excluded := []string{filepath.Join(root, "x"), glob}
	for name, scan := range map[string]func(context.Context, string, int, []string) ([]Repo, error){"Scan": Scan, "walkScan": walkScan} {
		for _, c := range []struct {
			depth    int
			excluded []string
			want     int
		}{{2, nil, 3}, {3, nil, 4}, {3, excluded, 2}} {
			repos, err := scan(context.Background(), root, c.depth, c.excluded)
			if err != nil {
				t.Fatal(err)
			}
			if len(repos) != c.want {
				t.Fatalf("%s at depth %d excluding %v found %v, want %d", name, c.depth, c.excluded, repos, c.want)
			}
		}
	}
	repos, _ := Scan(context.Background(), root, 3, nil)
	if top := MostActive(repos, 2, func(string) bool { return false }); len(top) != 2 || top[0].Path == a || top[0].Path == deep {
		t.Fatalf("MostActive returned %v, want a recent repo first", top)
	}
	// Pins come first, the most recently active win when they don't all fit
	if top := MostActive(repos, 2, func(p string) bool { return p == a }); len(top) != 2 || top[0].Path != a {
		t.Fatalf("MostActive returned %v, want pinned a first", top)
	}
	if top := MostActive(repos, 1, func(p string) bool { return p == a || p == b }); len(top) != 1 || top[0].Path != b {
		t.Fatalf("MostActive returned %v, want pinned b", top)
	}
}

func TestLsRemote(t *testing.T) {
	_, a, b := setup(t)
	run(t, a, "git", "commit", "--quiet", "--allow-empty", "-m", "two")
	run(t, a, "git", "push", "--quiet")
	check := func(want State, unknown bool) {
		t.Helper()
		r := Repo{Path: b}
		if err := r.LsRemote(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := r.Refresh(context.Background()); err != nil {
			t.Fatal(err)
		}
		if _, _, u := r.Counts(); r.State() != want || u != unknown {
			t.Fatalf("State %d unknown %t, want %d %t", r.State(), u, want, unknown)
		}
	}
	check(Behind, true) // Commit not fetched
	run(t, b, "git", "fetch", "--quiet")
	run(t, b, "git", "update-ref", "refs/remotes/origin/main", "HEAD")
	check(Behind, false) // Commit known, tracking ref stale
	run(t, b, "git", "pull", "--quiet", "--ff-only")
	r := Repo{Path: b}
	_ = r.LsRemote(context.Background())
	run(t, a, "git", "commit", "--quiet", "--allow-empty", "-m", "three")
	run(t, a, "git", "push", "--quiet")
	run(t, b, "git", "pull", "--quiet", "--ff-only")
	if err := r.Refresh(context.Background()); err != nil || r.State() != Synced {
		t.Fatalf("Stale tip gave state %d (%v), want synced", r.State(), err)
	}
}

func TestNoRemote(t *testing.T) {
	_, a, _ := setup(t)
	local := filepath.Join(t.TempDir(), "local")
	run(t, filepath.Dir(local), "git", "init", "--quiet", local)
	run(t, local, "git", "commit", "--quiet", "--allow-empty", "-m", "init")
	if r := state(t, local); r.HasRemote {
		t.Fatal("Repo without a remote has one")
	}
	if r := state(t, a); !r.HasRemote {
		t.Fatal("Clone has no remote")
	}
}

func TestFindRepos(t *testing.T) {
	root, a, b := setup(t)
	nested := filepath.Join(a, "sub", "nested")
	run(t, a, "git", "init", "--quiet", nested)
	repos := FindRepos([]string{a, root}, []string{a, root}, 3, time.Minute)
	var paths []string
	for _, r := range repos {
		paths = append(paths, r.Path)
	}
	// a as a root is taken whole, the search below root prunes it and finds b
	if !slices.Equal(paths, []string{a, b}) {
		t.Fatalf("Found %v", paths)
	}
	if repos[0].Name != "a" {
		t.Fatalf("Root repository named %q", repos[0].Name)
	}
}

func TestOnlyCheckedOutBranch(t *testing.T) {
	_, a, _ := setup(t)
	run(t, a, "git", "switch", "--quiet", "-c", "side")
	run(t, a, "git", "commit", "--quiet", "--allow-empty", "-m", "side")
	expect(t, a, Ahead)
	run(t, a, "git", "switch", "--quiet", "main")
	expect(t, a, Synced) // side has unpushed commits, but isn't checked out
	run(t, a, "git", "switch", "--quiet", "--detach")
	if r := state(t, a); r.Branch != nil || r.State() != Synced {
		t.Fatalf("Detached HEAD gave %+v", r.Branch)
	}
}
