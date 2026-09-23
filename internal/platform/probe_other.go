//go:build !windows && !linux

package platform

import "runtime"

// HostProbe queries the real host. On macOS and other systems no shared
// library probing is needed in v1.
type HostProbe struct{}

// OS returns runtime.GOOS.
func (HostProbe) OS() string { return runtime.GOOS }

// Arch returns runtime.GOARCH.
func (HostProbe) Arch() string { return runtime.GOARCH }

// HasLibrary always reports false; no backend on these systems needs it.
func (HostProbe) HasLibrary(string) bool { return false }

// GLibCVersion is not applicable.
func (HostProbe) GLibCVersion() string { return "" }
