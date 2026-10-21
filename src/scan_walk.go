package main

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
)

// walkScan is Scan without find: it walks the tree itself.
func walkScan(ctx context.Context, root string, depth int, excluded []string) ([]Repo, error) {
	var repos []Repo
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil || !d.IsDir() || path == root {
			// Unreadable directories are skipped, as find does
			return nil
		}
		if slices.Contains(excluded, path) {
			return fs.SkipDir
		}
		if d.Name() == ".git" {
			parent := filepath.Dir(path)
			name, _ := filepath.Rel(root, parent)
			repos = append(repos, Repo{Path: parent, Name: name})
			return fs.SkipDir
		}
		rel, _ := filepath.Rel(root, path)
		if strings.HasPrefix(d.Name(), ".") || strings.Count(rel, string(filepath.Separator)) >= depth {
			return fs.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("Unable to scan %s: %w", root, err)
	}
	return repos, nil
}
