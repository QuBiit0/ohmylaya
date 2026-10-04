package platform

import (
	"strings"
	"testing"
)

type fakeProbe struct {
	os, arch string
	libs     map[string]bool
	glibc    string
	macos    string
}

func (f fakeProbe) OS() string                  { return f.os }
func (f fakeProbe) Arch() string                { return f.arch }
func (f fakeProbe) HasLibrary(name string) bool { return f.libs[name] }
func (f fakeProbe) GLibCVersion() string        { return f.glibc }
func (f fakeProbe) MacOSVersion() string        { return f.macos }

func TestDetect(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		probe       fakeProbe
		wantOrder   []string
		wantDefault string
		wantCUDA    bool
	}{
		{
			name:        "windows with nvidia and vulkan",
			probe:       fakeProbe{os: "windows", arch: "amd64", libs: map[string]bool{"nvcuda.dll": true, "vulkan-1.dll": true}},
			wantOrder:   []string{"vulkan", "cuda", "cpu"},
			wantDefault: "vulkan",
			wantCUDA:    true,
		},
		{
			name:        "windows with vulkan only",
			probe:       fakeProbe{os: "windows", arch: "amd64", libs: map[string]bool{"vulkan-1.dll": true}},
			wantOrder:   []string{"vulkan", "cpu"},
			wantDefault: "vulkan",
		},
		{
			name:        "windows with the nvidia driver but no vulkan loader",
			probe:       fakeProbe{os: "windows", arch: "amd64", libs: map[string]bool{"nvcuda.dll": true}},
			wantOrder:   []string{"cuda"},
			wantDefault: "cuda",
			wantCUDA:    true,
		},
		{
			name:        "linux with cuda and vulkan and new glibc",
			probe:       fakeProbe{os: "linux", arch: "amd64", glibc: "2.40", libs: map[string]bool{"libcuda.so.1": true, "libvulkan.so.1": true}},
			wantOrder:   []string{"vulkan", "cuda", "cpu"},
			wantDefault: "vulkan",
			wantCUDA:    true,
		},
		{
			name:        "linux with old glibc offers nothing",
			probe:       fakeProbe{os: "linux", arch: "amd64", glibc: "2.35", libs: map[string]bool{"libvulkan.so.1": true}},
			wantOrder:   nil,
			wantDefault: "",
		},
		{
			name:        "linux arm64 unsupported by upstream",
			probe:       fakeProbe{os: "linux", arch: "arm64", glibc: "2.40"},
			wantOrder:   nil,
			wantDefault: "",
		},
		{
			name:        "macos 15 arm64 is cpu only in v1",
			probe:       fakeProbe{os: "darwin", arch: "arm64", macos: "15.0"},
			wantOrder:   []string{"cpu"},
			wantDefault: "cpu",
		},
		{
			name:        "macos with an unreadable version still gets cpu",
			probe:       fakeProbe{os: "darwin", arch: "arm64"},
			wantOrder:   []string{"cpu"},
			wantDefault: "cpu",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := Detect(tc.probe)
			got := make([]string, 0, len(d.Options))
			var sawCUDA bool
			for _, o := range d.Options {
				got = append(got, o.Backend)
				if o.Backend == "cuda" {
					sawCUDA = true
				}
				if o.Reason == "" {
					t.Errorf("option %s has no reason", o.Backend)
				}
			}
			if !equal(got, tc.wantOrder) {
				t.Errorf("options = %v, want %v", got, tc.wantOrder)
			}
			if d.Default != tc.wantDefault {
				t.Errorf("Default = %q, want %q", d.Default, tc.wantDefault)
			}
			if sawCUDA != tc.wantCUDA {
				t.Errorf("cuda offered = %v, want %v", sawCUDA, tc.wantCUDA)
			}
		})
	}
}

func TestDetectUnsupportedExplainsWhy(t *testing.T) {
	t.Parallel()
	d := Detect(fakeProbe{os: "linux", arch: "amd64", glibc: "2.31"})
	if d.Default != "" || len(d.Options) != 0 {
		t.Fatalf("expected no options, got %+v", d)
	}
	if d.Unsupported == "" {
		t.Error("Unsupported reason is empty")
	}
}

// The engine's CPU mode lives in the Vulkan build, which links the Vulkan
// loader, and the macOS build targets macOS 15. Without them the engine dies
// at start, after a large download, so detection must refuse up front.
func TestDetectRefusesHostsTheEngineCannotStartOn(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		probe fakeProbe
		want  []string
	}{
		"windows without vulkan loader": {fakeProbe{os: "windows", arch: "amd64"}, []string{"vulkan-1.dll", "GPU driver", "Vulkan Runtime"}},
		"linux without vulkan loader":   {fakeProbe{os: "linux", arch: "amd64", glibc: "2.40"}, []string{"libvulkan.so.1", "libvulkan1"}},
		"macos 14":                      {fakeProbe{os: "darwin", arch: "arm64", macos: "14.6"}, []string{"macOS 15", "14.6"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := Detect(tc.probe)
			if len(d.Options) != 0 || d.Default != "" {
				t.Fatalf("options = %+v, want none", d.Options)
			}
			for _, w := range tc.want {
				if !strings.Contains(d.Unsupported, w) {
					t.Errorf("Unsupported = %q, want it to mention %q", d.Unsupported, w)
				}
			}
		})
	}
}

func TestVersionAtLeast(t *testing.T) {
	t.Parallel()
	cases := []struct {
		have string
		want bool
	}{
		{"2.39", true}, {"2.40", true}, {"3.0", true}, {"3", true}, {"2.38", false}, {"2.9", false}, {"2", false}, {"", false}, {"garbage", false},
	}
	for _, tc := range cases {
		if got := versionAtLeast(tc.have, 2, 39); got != tc.want {
			t.Errorf("versionAtLeast(%q) = %v, want %v", tc.have, got, tc.want)
		}
	}
	if !versionAtLeast("15.0.1", 15, 0) || versionAtLeast("14.7.1", 15, 0) {
		t.Error("macOS versions compare by major then minor")
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
