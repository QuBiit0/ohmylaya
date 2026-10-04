//go:build darwin

package platform

import (
	"runtime"
	"syscall"
)

// HostProbe queries the real host. macOS needs no shared library probing in
// v1; the engine links only system frameworks.
type HostProbe struct{}

// OS returns runtime.GOOS.
func (HostProbe) OS() string { return runtime.GOOS }

// Arch returns runtime.GOARCH.
func (HostProbe) Arch() string { return runtime.GOARCH }

// HasLibrary always reports false; no macOS backend needs it.
func (HostProbe) HasLibrary(string) bool { return false }

// GLibCVersion is not applicable on macOS.
func (HostProbe) GLibCVersion() string { return "" }

// MacOSVersion reads the product version, such as "15.1", from sysctl.
func (HostProbe) MacOSVersion() string {
	v, err := syscall.Sysctl("kern.osproductversion")
	if err != nil {
		return ""
	}
	return v
}
