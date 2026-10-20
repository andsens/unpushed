package main

import "os/exec"

// terminal opens the user's pick in xdg-terminals.list in dir.
func terminal(dir string) *exec.Cmd {
	return exec.Command("xdg-terminal-exec", "--dir="+dir)
}
