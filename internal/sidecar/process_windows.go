//go:build windows

package sidecar

import (
	"os"
	"os/exec"
	"syscall"
)

const (
	createNewProcessGroup = 0x00000200
	detachedProcess       = 0x00000008
	stillActive           = 259
	processQueryLimited   = 0x1000
)

// detach makes the child survive the parent and not share its console.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup | detachedProcess}
}

// alive reports whether pid is a running process.
func alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := syscall.OpenProcess(processQueryLimited, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	var code uint32
	if err := syscall.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}

// terminate ends the process. Windows has no SIGTERM for a detached
// process; the caller drains the queue before calling this.
func terminate(p *os.Process) error {
	return p.Kill()
}
