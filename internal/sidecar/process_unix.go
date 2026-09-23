//go:build !windows

package sidecar

import (
	"os"
	"os/exec"
	"syscall"
)

// detach puts the child in its own session so it survives the parent.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// alive reports whether pid is a running process we may signal.
func alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// terminate asks the engine to drain and exit.
func terminate(p *os.Process) error {
	return p.Signal(syscall.SIGTERM)
}
