package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type State int

const (
	Synced State = iota
	Behind
	Ahead
	Failed
)

type Branch struct {
	Name      string
	Head      bool
	Upstream  string // refs/remotes/<remote>/<branch>, empty without one
	Remote    string
	RemoteRef string
	Gone      bool
	Ahead     int
	Behind    int
	Unknown   bool // The remote moved to a commit that hasn't been fetched
}

type Repo struct {
	Path     string
	Name     string
	Active   time.Time
	Branches []Branch
	// Repos without a remote have nothing to push to and aren't listed
	HasRemote bool
	// Branch tips from ls-remote keyed by <remote>\x00<ref>, nil when the
	// remote isn't checked
	RemoteHeads map[string]string
}

// Scan finds repositories (directories with a .git directory) at most depth
// levels below root, skipping hidden directories. find runs at idle priority,
// a home directory is a lot of inodes.
func Scan(ctx context.Context, root string, depth int) ([]Repo, error) {
	cmd := exec.CommandContext(ctx, "nice", "-n19", "ionice", "-c3",
		"find", root, "-mindepth", "1", "-maxdepth", strconv.Itoa(depth+1),
		"(", "-name", ".*", "!", "-name", ".git", ")", "-prune",
		"-o", "-name", ".git", "-type", "d", "-print", "-prune")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		// find exits 1 on unreadable directories but still prints the rest
		if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != 1 {
			return nil, fmt.Errorf("Unable to scan %s: %w: %s", root, err, strings.TrimSpace(stderr.String()))
		}
		slog.Debug("find reported errors", "root", root, "stderr", strings.TrimSpace(stderr.String()))
	}
	var repos []Repo
	for line := range strings.Lines(string(out)) {
		path := filepath.Dir(strings.TrimSuffix(line, "\n"))
		name, _ := filepath.Rel(root, path)
		repos = append(repos, Repo{Path: path, Name: name})
	}
	return repos, nil
}

// MostActive sorts repos by last activity and keeps the first max.
func MostActive(repos []Repo, max int) []Repo {
	for i := range repos {
		repos[i].Active = activity(filepath.Join(repos[i].Path, ".git"))
	}
	sort.SliceStable(repos, func(i, j int) bool { return repos[i].Active.After(repos[j].Active) })
	if len(repos) > max {
		repos = repos[:max]
	}
	return repos
}

// The HEAD reflog is appended to on commit, checkout, merge, rebase and reset.
func activity(gitDir string) time.Time {
	for _, name := range []string{"logs/HEAD", "HEAD"} {
		if fi, err := os.Stat(filepath.Join(gitDir, name)); err == nil {
			return fi.ModTime()
		}
	}
	return time.Time{}
}

func git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s in %s: %w: %s", strings.Join(args, " "), dir, err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (r *Repo) Refresh(ctx context.Context) error {
	remotes, err := git(ctx, r.Path, "remote")
	if err != nil {
		return err
	}
	r.HasRemote = remotes != ""
	out, err := git(ctx, r.Path, "for-each-ref", "--format=%(refname:short)%00%(HEAD)%00%(upstream)%00%(upstream:remotename)%00%(upstream:remoteref)", "refs/heads")
	if err != nil {
		return err
	}
	r.Branches = nil
	for line := range strings.Lines(out) {
		f := strings.Split(strings.TrimSuffix(line, "\n"), "\x00")
		b := Branch{Name: f[0], Head: f[1] == "*", Upstream: f[2], Remote: f[3], RemoteRef: f[4]}
		if b.Upstream != "" {
			if _, err := git(ctx, r.Path, "rev-parse", "--verify", "--quiet", b.Upstream); err != nil {
				b.Gone = true
			}
		}
		var counts string
		if b.Upstream != "" && !b.Gone {
			counts, err = git(ctx, r.Path, "rev-list", "--left-right", "--count", "refs/heads/"+b.Name+"..."+b.Upstream)
		} else {
			// Without an upstream, only commits no remote has count as unpushed
			counts, err = git(ctx, r.Path, "rev-list", "--count", "refs/heads/"+b.Name, "--not", "--remotes")
			counts += "\t0"
		}
		if err != nil {
			return err
		}
		ahead, behind, _ := strings.Cut(counts, "\t")
		b.Ahead, _ = strconv.Atoi(ahead)
		b.Behind, _ = strconv.Atoi(behind)
		// A tip the upstream already contains is older than the last fetch
		if tip, ok := r.RemoteHeads[b.Remote+"\x00"+b.RemoteRef]; ok && b.Upstream != "" && !b.Gone {
			if _, err := git(ctx, r.Path, "cat-file", "-e", tip+"^{commit}"); err != nil {
				b.Unknown = true
			} else if _, err := git(ctx, r.Path, "merge-base", "--is-ancestor", tip, b.Upstream); err != nil {
				counts, err := git(ctx, r.Path, "rev-list", "--left-right", "--count", "refs/heads/"+b.Name+"..."+tip)
				if err != nil {
					return err
				}
				ahead, behind, _ := strings.Cut(counts, "\t")
				b.Ahead, _ = strconv.Atoi(ahead)
				b.Behind, _ = strconv.Atoi(behind)
			}
		}
		r.Branches = append(r.Branches, b)
	}
	return nil
}

// LsRemote records the branch tips of every remote without fetching.
func (r *Repo) LsRemote(ctx context.Context) error {
	out, err := git(ctx, r.Path, "remote")
	if err != nil {
		return err
	}
	heads := map[string]string{}
	for _, remote := range strings.Fields(out) {
		out, err := git(ctx, r.Path, "ls-remote", "--heads", remote)
		if err != nil {
			return err
		}
		for line := range strings.Lines(out) {
			if sha, ref, ok := strings.Cut(strings.TrimSpace(line), "\t"); ok {
				heads[remote+"\x00"+ref] = sha
			}
		}
	}
	r.RemoteHeads = heads
	return nil
}

func (b Branch) State() State {
	switch {
	case b.Ahead > 0 && (b.Behind > 0 || b.Unknown || b.Gone):
		return Failed
	case b.Ahead > 0:
		return Ahead
	case b.Behind > 0 || b.Unknown:
		return Behind
	}
	return Synced
}

func (r Repo) State() State {
	s := Synced
	for _, b := range r.Branches {
		s = max(s, b.State())
	}
	return s
}

func (r Repo) Counts() (ahead, behind int, unknown bool) {
	for _, b := range r.Branches {
		ahead += b.Ahead
		behind += b.Behind
		unknown = unknown || b.Unknown
	}
	return
}

func (r Repo) Fetch(ctx context.Context) error {
	_, err := git(ctx, r.Path, "fetch", "--all", "--quiet")
	return err
}

// Sync pushes branches that are ahead and fast-forwards branches that are
// behind, after fetching.
func (r *Repo) Sync(ctx context.Context) error {
	if err := r.Fetch(ctx); err != nil {
		return err
	}
	if err := r.Refresh(ctx); err != nil {
		return err
	}
	var errs []error
	for _, b := range r.Branches {
		var err error
		switch b.State() {
		case Failed:
			err = fmt.Errorf("Branch %s has diverged from or lost its upstream", b.Name)
		case Ahead:
			if b.Upstream != "" {
				_, err = git(ctx, r.Path, "push", "--quiet", b.Remote, "refs/heads/"+b.Name+":"+b.RemoteRef)
			} else if remote, rerr := soleRemote(ctx, r.Path); rerr != nil {
				err = fmt.Errorf("Branch %s has no upstream: %w", b.Name, rerr)
			} else {
				_, err = git(ctx, r.Path, "push", "--quiet", "--set-upstream", remote, b.Name)
			}
		case Behind:
			if b.Head {
				_, err = git(ctx, r.Path, "merge", "--ff-only", "--quiet", b.Upstream)
			} else {
				// A non-forced fetch into a local branch only fast-forwards
				_, err = git(ctx, r.Path, "fetch", "--quiet", ".", b.Upstream+":refs/heads/"+b.Name)
			}
		}
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func soleRemote(ctx context.Context, dir string) (string, error) {
	out, err := git(ctx, dir, "remote")
	if err != nil {
		return "", err
	}
	remotes := strings.Fields(out)
	if len(remotes) != 1 {
		return "", fmt.Errorf("Expected exactly one remote, found %d", len(remotes))
	}
	return remotes[0], nil
}
