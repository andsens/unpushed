package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Scan finds repositories (directories with a .git directory) at most depth
// levels below root, skipping hidden and pruned directories. find runs at
// idle priority, a home directory is a lot of inodes.
func Scan(ctx context.Context, root string, depth int, pruned []string) ([]Repo, error) {
	args := []string{"-n19", "ionice", "-c3", "find", root, "-mindepth", "1", "-maxdepth", strconv.Itoa(depth + 1)}
	for _, path := range pruned {
		// -path takes a glob
		args = append(args, "-path", globEscape.Replace(path), "-prune", "-o")
	}
	args = append(args, "(", "-name", ".*", "!", "-name", ".git", ")", "-prune",
		"-o", "-name", ".git", "-type", "d", "-print", "-prune")
	cmd := exec.CommandContext(ctx, "nice", args...)
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

var globEscape = strings.NewReplacer(`\`, `\\`, "*", `\*`, "?", `\?`, "[", `\[`)
