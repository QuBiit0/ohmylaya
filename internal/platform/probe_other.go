//go:build !windows && !linux && !darwin

package platform

import "runtime"

// HostProbe queries the real host. No engine build exists for these
// systems, so Detect reports them unsupported.
type HostProbe struct{}

// OS returns runtime.GOOS.
func (HostProbe) OS() string { return runtime.GOOS }

// Arch returns runtime.GOARCH.
func (HostProbe) Arch() string { return runtime.GOARCH }

// HasLibrary always reports false; no backend on these systems needs it.
func (HostProbe) HasLibrary(string) bool { return false }

// GLibCVersion is not applicable.
func (HostProbe) GLibCVersion() string { return "" }

// MacOSVersion is not applicable.
func (HostProbe) MacOSVersion() string { return "" }
