//go:build !linux

package main

import "context"

// Scan finds repositories (directories with a .git directory) at most depth
// levels below root, skipping hidden and excluded directories.
func Scan(ctx context.Context, root string, depth int, excluded []string) ([]Repo, error) {
	return walkScan(ctx, root, depth, excluded)
}
