package main

import "os/exec"

// terminal opens Windows Terminal in dir, or a Command Prompt where it's
// missing.
func terminal(dir string) *exec.Cmd {
	if _, err := exec.LookPath("wt"); err == nil {
		return exec.Command("wt", "-d", dir)
	}
	// The first quoted argument to start is the window title
	return exec.Command("cmd", "/c", "start", "unpushed", "/d", dir, "cmd")
}
