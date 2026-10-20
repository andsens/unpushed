package main

import "os/exec"

// terminal opens Terminal.app in dir.
func terminal(dir string) *exec.Cmd {
	return exec.Command("open", "-a", "Terminal", dir)
}
