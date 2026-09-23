package manifest

import (
	"errors"
	"strings"
	"testing"
)

func TestEmbeddedManifestLoadsAndValidates(t *testing.T) {
	t.Parallel()
	m, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if m.Schema != 1 {
		t.Errorf("Schema = %d, want 1", m.Schema)
	}
	if m.Engine.Tag == "" {
		t.Error("Engine.Tag is empty")
	}
	if len(m.Engine.Assets) != 5 {
		t.Errorf("len(Engine.Assets) = %d, want 5", len(m.Engine.Assets))
	}
	if len(m.Models.Variants) != 3 {
		t.Errorf("len(Models.Variants) = %d, want 3", len(m.Models.Variants))
	}
}

func TestEngineAssetLookup(t *testing.T) {
	t.Parallel()
	m := mustLoad(t)

	cases := []struct {
		os, arch, backend string
		wantName          string
		wantErr           error
	}{
		{"windows", "amd64", "vulkan", "laya-r0002-windows-amd64-vulkan.exe", nil},
		{"windows", "amd64", "cuda", "laya-r0002-windows-amd64-cuda.exe", nil},
		{"linux", "amd64", "vulkan", "laya-r0002-linux-amd64-vulkan", nil},
		{"darwin", "arm64", "coreml", "laya-r0002-macos-arm64-coreml", nil},
		// cpu is served by whichever asset supports it on the platform.
		{"windows", "amd64", "cpu", "laya-r0002-windows-amd64-vulkan.exe", nil},
		{"darwin", "arm64", "cpu", "laya-r0002-macos-arm64-coreml", nil},
		{"linux", "arm64", "vulkan", "", ErrNoAsset},
		{"plan9", "amd64", "cpu", "", ErrNoAsset},
	}
	for _, tc := range cases {
		t.Run(tc.os+"/"+tc.arch+"/"+tc.backend, func(t *testing.T) {
			t.Parallel()
			a, err := m.EngineAsset(tc.os, tc.arch, tc.backend)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if err == nil && a.Name != tc.wantName {
				t.Errorf("Name = %q, want %q", a.Name, tc.wantName)
			}
		})
	}
}

func TestWindowsCUDAAssetCarriesCublasExtra(t *testing.T) {
	t.Parallel()
	m := mustLoad(t)
	a, err := m.EngineAsset("windows", "amd64", "cuda")
	if err != nil {
		t.Fatal(err)
	}
	if a.Extra == nil {
		t.Fatal("Extra is nil, want cuBLAS archive")
	}
	if len(a.Extra.Members) < 2 {
		t.Errorf("Extra.Members = %v, want the two cuBLAS DLLs", a.Extra.Members)
	}
	if v, _ := m.EngineAsset("windows", "amd64", "vulkan"); v.Extra != nil {
		t.Error("vulkan asset must not carry an extra archive")
	}
}

func TestVariantLookupAndFileURLs(t *testing.T) {
	t.Parallel()
	m := mustLoad(t)

	v, err := m.Variant("multilingual")
	if err != nil {
		t.Fatal(err)
	}
	if v.Context != 1024 || v.HeadMaxLen != 256 {
		t.Errorf("multilingual budget = %d/%d, want 1024/256", v.Context, v.HeadMaxLen)
	}
	url := m.FileURL(v, v.Files[0])
	want := "https://huggingface.co/convaiinnovations/laya/resolve/1c5edc17a7acd8701df6fc341c0d179f1c62c982/multilingual/model.safetensors"
	if url != want {
		t.Errorf("FileURL = %q, want %q", url, want)
	}

	e, _ := m.Variant("english")
	if got := m.FileURL(e, e.Files[0]); !strings.HasSuffix(got, "/resolve/1c5edc17a7acd8701df6fc341c0d179f1c62c982/model.safetensors") {
		t.Errorf("english root file URL = %q", got)
	}

	if _, err := m.Variant("klingon"); !errors.Is(err, ErrNoVariant) {
		t.Errorf("unknown variant err = %v, want ErrNoVariant", err)
	}
}

func TestVariantTotalSize(t *testing.T) {
	t.Parallel()
	m := mustLoad(t)
	v, _ := m.Variant("multilingual")
	const want = 643835514 + 472 + 1938 + 34363188 + 524
	if got := v.TotalSize(); got != want {
		t.Errorf("TotalSize = %d, want %d", got, want)
	}
}

func TestValidateRejectsBrokenManifests(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		mut  func(*Manifest)
		want string
	}{
		{"bad schema", func(m *Manifest) { m.Schema = 2 }, "schema"},
		{"short digest", func(m *Manifest) { m.Engine.Assets[0].SHA256 = "abc" }, "sha256"},
		{"zero size", func(m *Manifest) { m.Models.Variants["english"].Files[0].Size = 0 }, "size"},
		{"empty revision", func(m *Manifest) { m.Models.Revision = "" }, "revision"},
		{"duplicate asset", func(m *Manifest) { m.Engine.Assets = append(m.Engine.Assets, m.Engine.Assets[0]) }, "duplicate"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := mustLoad(t)
			tc.mut(m)
			err := m.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Validate() = %v, want error containing %q", err, tc.want)
			}
		})
	}
}

func mustLoad(t *testing.T) *Manifest {
	t.Helper()
	m, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	return m
}
