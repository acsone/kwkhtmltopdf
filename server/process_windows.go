//go:build windows
// +build windows

package main

import "os/exec"

// process groups are set up differently here, so we only handle the child
func setProcessGroup(cmd *exec.Cmd) {
}

func killProcessTree(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	cmd.Process.Kill()
}
