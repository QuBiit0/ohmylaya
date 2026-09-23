//go:build windows

package platform

import (
	"runtime"
	"syscall"
)

// HostProbe queries the real host.
type HostProbe struct{}

// OS returns runtime.GOOS.
func (HostProbe) OS() string { return runtime.GOOS }

// Arch returns runtime.GOARCH.
func (HostProbe) Arch() string { return runtime.GOARCH }

// HasLibrary tries to load the DLL through the system search path, which is
// what the engine itself will do, then frees it.
func (HostProbe) HasLibrary(name string) bool {
	h, err := syscall.LoadLibrary(name)
	if err != nil {
		return false
	}
	_ = syscall.FreeLibrary(h)
	return true
}

// GLibCVersion is not applicable on Windows.
func (HostProbe) GLibCVersion() string { return "" }
