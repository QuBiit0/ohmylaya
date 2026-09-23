//go:build linux

package platform

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// HostProbe queries the real host.
type HostProbe struct{}

// OS returns runtime.GOOS.
func (HostProbe) OS() string { return runtime.GOOS }

// Arch returns runtime.GOARCH.
func (HostProbe) Arch() string { return runtime.GOARCH }

// libraryDirs are the usual loader search paths on glibc distributions.
var libraryDirs = []string{
	"/usr/lib/x86_64-linux-gnu",
	"/lib/x86_64-linux-gnu",
	"/usr/lib64",
	"/lib64",
	"/usr/lib",
	"/lib",
	"/usr/local/lib",
	"/usr/lib/wsl/lib",
}

// HasLibrary looks for the shared object in the loader paths and in
// LD_LIBRARY_PATH. It does not dlopen, so a broken driver still counts as
// present; the smoke test catches that later.
func (HostProbe) HasLibrary(name string) bool {
	dirs := append([]string{}, libraryDirs...)
	if extra := os.Getenv("LD_LIBRARY_PATH"); extra != "" {
		dirs = append(strings.Split(extra, ":"), dirs...)
	}
	for _, dir := range dirs {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

// GLibCVersion asks getconf, which every glibc system ships.
func (HostProbe) GLibCVersion() string {
	out, err := exec.Command("getconf", "GNU_LIBC_VERSION").Output()
	if err != nil {
		return ""
	}
	// Output looks like "glibc 2.39".
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return ""
	}
	return fields[1]
}
