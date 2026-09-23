// Package manifest pins the exact engine executables and model files that a
// given ohmylaya release installs. The manifest is embedded in the binary, so
// the whole stack is reproducible from the ohmylaya version alone.
package manifest

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

//go:embed manifest.json
var embedded []byte

// Sentinel errors returned by lookups.
var (
	ErrNoAsset   = errors.New("manifest: no engine asset for platform")
	ErrNoVariant = errors.New("manifest: unknown model variant")
)

// Manifest is the top-level pinned dependency set.
type Manifest struct {
	Schema    int    `json:"schema"`
	Engine    Engine `json:"engine"`
	Models    Models `json:"models"`
	Signature string `json:"signature"`
}

// Engine describes one laya.cpp release and its per-platform executables.
type Engine struct {
	Project string  `json:"project"`
	Tag     string  `json:"tag"`
	Assets  []Asset `json:"assets"`
}

// Asset is one downloadable engine executable.
type Asset struct {
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	Backend     string `json:"backend"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	SupportsCPU bool   `json:"supports_cpu"`
	Extra       *Extra `json:"extra,omitempty"`
}

// Extra is an auxiliary archive an asset needs at runtime, such as the
// Windows cuBLAS DLLs, of which only the listed members are extracted.
type Extra struct {
	Name    string   `json:"name"`
	URL     string   `json:"url"`
	SHA256  string   `json:"sha256"`
	Members []string `json:"members"`
}

// Models describes the Hugging Face repository and its variants.
type Models struct {
	Repo     string              `json:"repo"`
	Revision string              `json:"revision"`
	Variants map[string]*Variant `json:"variants"`
}

// Variant is one checkpoint. Prefix is the path inside the repository under
// which its files live; it is empty for the repository root.
type Variant struct {
	Prefix     string `json:"prefix"`
	Context    int    `json:"context"`
	HeadMaxLen int    `json:"head_max_len"`
	Files      []File `json:"files"`
}

// File is one checkpoint file relative to the variant directory.
type File struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// TotalSize returns the sum of all file sizes in bytes.
func (v *Variant) TotalSize() int64 {
	var n int64
	for _, f := range v.Files {
		n += f.Size
	}
	return n
}

// Load parses and validates the embedded manifest.
func Load() (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(embedded, &m); err != nil {
		return nil, fmt.Errorf("manifest: parse embedded: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// EngineAsset returns the executable for the platform and backend. The "cpu"
// backend resolves to any asset for the platform that supports CPU execution,
// preferring the smallest download.
func (m *Manifest) EngineAsset(goos, goarch, backend string) (*Asset, error) {
	var best *Asset
	for i := range m.Engine.Assets {
		a := &m.Engine.Assets[i]
		if a.OS != goos || a.Arch != goarch {
			continue
		}
		if a.Backend == backend {
			return a, nil
		}
		if backend == "cpu" && a.SupportsCPU && (best == nil || a.Size < best.Size) {
			best = a
		}
	}
	if best != nil {
		return best, nil
	}
	return nil, fmt.Errorf("%w: %s/%s backend %q", ErrNoAsset, goos, goarch, backend)
}

// Variant returns the named checkpoint.
func (m *Manifest) Variant(name string) (*Variant, error) {
	v, ok := m.Models.Variants[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrNoVariant, name)
	}
	return v, nil
}

// FileURL returns the direct download URL for a checkpoint file at the pinned
// revision.
func (m *Manifest) FileURL(v *Variant, f File) string {
	return "https://huggingface.co/" + m.Models.Repo + "/resolve/" + m.Models.Revision + "/" + v.Prefix + f.Path
}

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Validate checks structural invariants so a broken manifest fails at build
// and test time rather than during a user's install.
func (m *Manifest) Validate() error {
	if m.Schema != 1 {
		return fmt.Errorf("manifest: unsupported schema %d", m.Schema)
	}
	if m.Engine.Tag == "" || m.Engine.Project == "" {
		return errors.New("manifest: engine tag and project are required")
	}
	seen := map[string]bool{}
	for _, a := range m.Engine.Assets {
		key := a.OS + "/" + a.Arch + "/" + a.Backend
		if seen[key] {
			return fmt.Errorf("manifest: duplicate asset %s", key)
		}
		seen[key] = true
		if err := checkDigest(a.Name, a.Size, a.SHA256); err != nil {
			return err
		}
		if a.URL == "" {
			return fmt.Errorf("manifest: asset %s has no url", a.Name)
		}
		if a.Extra != nil {
			if !sha256Pattern.MatchString(a.Extra.SHA256) || a.Extra.URL == "" || len(a.Extra.Members) == 0 {
				return fmt.Errorf("manifest: asset %s has an invalid extra archive", a.Name)
			}
		}
	}
	if m.Models.Repo == "" || m.Models.Revision == "" {
		return errors.New("manifest: models repo and revision are required")
	}
	for name, v := range m.Models.Variants {
		if v.Context <= 0 || v.HeadMaxLen <= 0 || v.HeadMaxLen >= v.Context {
			return fmt.Errorf("manifest: variant %s has an invalid token budget", name)
		}
		if len(v.Files) == 0 {
			return fmt.Errorf("manifest: variant %s has no files", name)
		}
		for _, f := range v.Files {
			if err := checkDigest(name+"/"+f.Path, f.Size, f.SHA256); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkDigest(name string, size int64, sum string) error {
	if size <= 0 {
		return fmt.Errorf("manifest: %s has an invalid size %d", name, size)
	}
	if !sha256Pattern.MatchString(sum) {
		return fmt.Errorf("manifest: %s has an invalid sha256 %q", name, sum)
	}
	return nil
}
