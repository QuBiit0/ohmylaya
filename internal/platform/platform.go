// Package platform detects the host and recommends an engine backend.
package platform

import (
	"fmt"
	"strconv"
	"strings"
)

// Probe abstracts the host queries so detection is testable without a GPU.
type Probe interface {
	OS() string
	Arch() string
	// HasLibrary reports whether a shared library can be resolved by name.
	HasLibrary(name string) bool
	// GLibCVersion returns "major.minor" on Linux and "" elsewhere.
	GLibCVersion() string
}

// Option is one backend the host can run.
type Option struct {
	Backend string
	Reason  string
}

// Detection is the ordered list of runnable backends plus the recommended
// default. Options are ordered by preference. Unsupported is set when no
// backend can run and explains why.
type Detection struct {
	OS          string
	Arch        string
	Options     []Option
	Default     string
	Unsupported string
}

const (
	minGLibCMajor = 2
	minGLibCMinor = 39
)

// Detect applies the backend table from the installer spec.
func Detect(p Probe) Detection {
	d := Detection{OS: p.OS(), Arch: p.Arch()}

	switch {
	case d.OS == "windows" && d.Arch == "amd64":
		if p.HasLibrary("vulkan-1.dll") {
			d.add("vulkan", "Vulkan loader found; works on NVIDIA, AMD and Intel GPUs, about 80 MB")
		}
		if p.HasLibrary("nvcuda.dll") {
			d.add("cuda", "NVIDIA driver found; fastest on NVIDIA, about 200 MB plus two cuBLAS DLLs")
		}
		d.add("cpu", "always available; expect hundreds of milliseconds per question")

	case d.OS == "linux" && d.Arch == "amd64":
		glibc := p.GLibCVersion()
		if !glibcAtLeast(glibc, minGLibCMajor, minGLibCMinor) {
			d.Unsupported = fmt.Sprintf("upstream engine builds need glibc %d.%d or newer (found %q)", minGLibCMajor, minGLibCMinor, glibc)
			return d
		}
		if p.HasLibrary("libvulkan.so.1") {
			d.add("vulkan", "Vulkan loader found; works on NVIDIA, AMD and Intel GPUs, about 82 MB")
		}
		if p.HasLibrary("libcuda.so.1") {
			d.add("cuda", "NVIDIA driver found; fastest on NVIDIA, about 786 MB")
		}
		d.add("cpu", "always available; expect hundreds of milliseconds per question")

	case d.OS == "darwin" && d.Arch == "arm64":
		d.add("cpu", "Core ML needs exported model buckets that are not published yet; CPU is the v1 backend on macOS")

	default:
		d.Unsupported = fmt.Sprintf("no upstream engine build for %s/%s", d.OS, d.Arch)
		return d
	}

	if len(d.Options) > 0 {
		d.Default = d.Options[0].Backend
	}
	return d
}

func (d *Detection) add(backend, reason string) {
	d.Options = append(d.Options, Option{Backend: backend, Reason: reason})
}

// glibcAtLeast parses "major.minor" and compares it with the minimum.
func glibcAtLeast(version string, major, minor int) bool {
	parts := strings.SplitN(strings.TrimSpace(version), ".", 3)
	if len(parts) < 2 {
		return false
	}
	maj, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}
	min, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	if maj != major {
		return maj > major
	}
	return min >= minor
}
