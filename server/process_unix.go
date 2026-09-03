//go:build !windows
// +build !windows

package main

import (
	"os/exec"
	"syscall"
)

// run the renderer in its own process group, so we can kill its children too
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killing the group matters: children keep the stdout pipe open, and the
// server stays blocked on it until the last one is gone
func killProcessTree(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		cmd.Process.Kill()
	}
}
