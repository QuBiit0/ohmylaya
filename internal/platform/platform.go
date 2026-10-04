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
	// MacOSVersion returns the product version such as "15.1" on macOS, and
	// "" elsewhere or when it cannot be read.
	MacOSVersion() string
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
	// minMacOSMajor is the deployment target of the upstream macOS build
	// (LC_BUILD_VERSION minos in laya.cpp r0002).
	minMacOSMajor = 15
)

// VulkanLoader names the Vulkan loader library on goos and how to install
// it. The upstream CPU mode lives in the Vulkan build, which links the loader
// dynamically, so the CPU backend needs it too. It returns empty strings on
// systems whose engine does not use Vulkan.
func VulkanLoader(goos string) (lib, fix string) {
	switch goos {
	case "windows":
		return "vulkan-1.dll", "install or update your GPU driver, or install the Vulkan Runtime from https://vulkan.lunarg.com/sdk/home"
	case "linux":
		return "libvulkan.so.1", "install the Vulkan loader: libvulkan1 (Debian, Ubuntu) or vulkan-loader (Fedora, Arch)"
	}
	return "", ""
}

func noLoader(goos string) string {
	lib, fix := VulkanLoader(goos)
	return "the engine needs the Vulkan loader (" + lib + "), even on the CPU backend; " + fix
}

// Detect applies the backend table from the installer spec.
func Detect(p Probe) Detection {
	d := Detection{OS: p.OS(), Arch: p.Arch()}

	switch {
	case d.OS == "windows" && d.Arch == "amd64":
		lib, _ := VulkanLoader(d.OS)
		loader := p.HasLibrary(lib)
		if loader {
			d.add("vulkan", "Vulkan loader found; works on NVIDIA, AMD and Intel GPUs, about 80 MB")
		}
		if p.HasLibrary("nvcuda.dll") {
			d.add("cuda", "NVIDIA driver found; fastest on NVIDIA, about 200 MB plus two cuBLAS DLLs")
		}
		if loader {
			d.add("cpu", "runs the Vulkan build without a GPU; expect hundreds of milliseconds per question")
		}
		if len(d.Options) == 0 {
			d.Unsupported = noLoader(d.OS)
			return d
		}

	case d.OS == "linux" && d.Arch == "amd64":
		glibc := p.GLibCVersion()
		if !versionAtLeast(glibc, minGLibCMajor, minGLibCMinor) {
			d.Unsupported = fmt.Sprintf("upstream engine builds need glibc %d.%d or newer (found %q)", minGLibCMajor, minGLibCMinor, glibc)
			return d
		}
		lib, _ := VulkanLoader(d.OS)
		loader := p.HasLibrary(lib)
		if loader {
			d.add("vulkan", "Vulkan loader found; works on NVIDIA, AMD and Intel GPUs, about 82 MB")
		}
		if p.HasLibrary("libcuda.so.1") {
			d.add("cuda", "NVIDIA driver found; fastest on NVIDIA, about 786 MB")
		}
		if loader {
			d.add("cpu", "runs the Vulkan build without a GPU; expect hundreds of milliseconds per question")
		}
		if len(d.Options) == 0 {
			d.Unsupported = noLoader(d.OS)
			return d
		}

	case d.OS == "darwin" && d.Arch == "arm64":
		// An unreadable version is not proof of an old system; let the
		// install smoke test have the last word.
		if v := p.MacOSVersion(); v != "" && !versionAtLeast(v, minMacOSMajor, 0) {
			d.Unsupported = fmt.Sprintf("the upstream engine needs macOS %d or newer (found %s)", minMacOSMajor, v)
			return d
		}
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

// versionAtLeast parses "major[.minor[.patch]]" and compares it with the
// minimum. A missing minor reads as zero.
func versionAtLeast(version string, major, minor int) bool {
	parts := strings.SplitN(strings.TrimSpace(version), ".", 3)
	maj, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}
	min := 0
	if len(parts) > 1 {
		if min, err = strconv.Atoi(parts[1]); err != nil {
			return false
		}
	}
	if maj != major {
		return maj > major
	}
	return min >= minor
}
