package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuBiit0/ohmylaya/internal/manifest"
)

const newRev = "abcdefabcdefabcdefabcdefabcdefabcdefabcd"

func sum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// upstream fakes a GitHub release and a Hugging Face repository. Tests break
// one of its maps to simulate an inconsistent upstream.
type upstream struct {
	assets map[string]string // release asset name -> content
	sums   map[string]string // SHA256SUMS entries, name -> digest
	digest map[string]string // GitHub API digest, name -> digest
	files  map[string]string // repository path -> content
	lfs    map[string]bool   // repository paths stored in LFS
	oid    map[string]string // git blob oid overrides
}

func newUpstream() *upstream {
	u := &upstream{
		assets: map[string]string{
			"laya-r0002-linux-amd64-vulkan":     "linux engine",
			"laya-r0002-windows-amd64-cuda.exe": "windows engine",
		},
		sums: map[string]string{}, digest: map[string]string{}, oid: map[string]string{},
		files: map[string]string{
			"multilingual/model.safetensors":   "weights",
			"multilingual/encoder/config.json": `{"hidden":8}`,
			"english/model.safetensors":        "other variant",
		},
		lfs: map[string]bool{"multilingual/model.safetensors": true, "english/model.safetensors": true},
	}
	for name, body := range u.assets {
		u.sums[name], u.digest[name] = sum(body), sum(body)
	}
	return u
}

func (u *upstream) serve(t *testing.T) Sources {
	t.Helper()
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/up/engine/releases/tags/r0002", func(w http.ResponseWriter, _ *http.Request) {
		var rel ghRelease
		for name, body := range u.assets {
			rel.Assets = append(rel.Assets, ghAsset{name, int64(len(body)), "sha256:" + u.digest[name], srv.URL + "/dl/" + name})
		}
		rel.Assets = append(rel.Assets, ghAsset{Name: "SHA256SUMS", URL: srv.URL + "/dl/SHA256SUMS"})
		_ = json.NewEncoder(w).Encode(rel)
	})
	mux.HandleFunc("GET /dl/{name}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("name") != "SHA256SUMS" {
			http.Error(w, "not needed", http.StatusTeapot)
			return
		}
		for name, d := range u.sums {
			_, _ = w.Write([]byte(d + "  " + name + "\n"))
		}
	})
	mux.HandleFunc("GET /api/models/up/model/revision/main", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"sha":"` + newRev + `"}`))
	})
	mux.HandleFunc("GET /api/models/up/model/tree/{rev}/{prefix...}", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("recursive") != "true" {
			http.Error(w, "want recursive", http.StatusBadRequest)
			return
		}
		var tree []hfEntry
		for path, body := range u.files {
			if !strings.HasPrefix(path, r.PathValue("prefix")) {
				continue
			}
			e := hfEntry{Type: "file", Path: path, Size: int64(len(body)), OID: blobOID([]byte(body))}
			if o, ok := u.oid[path]; ok {
				e.OID = o
			}
			if u.lfs[path] {
				e.LFS = &hfLFS{OID: sum(body), Size: int64(len(body))}
			}
			tree = append(tree, e)
		}
		_ = json.NewEncoder(w).Encode(tree)
	})
	mux.HandleFunc("GET /up/model/resolve/{rev}/{path...}", func(w http.ResponseWriter, r *http.Request) {
		if u.lfs[r.PathValue("path")] {
			http.Error(w, "large files are never downloaded", http.StatusTeapot)
			return
		}
		_, _ = w.Write([]byte(u.files[r.PathValue("path")]))
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return Sources{Client: srv.Client(), GitHubAPI: srv.URL, HFBase: srv.URL}
}

func loadTemplate(t *testing.T) *manifest.Manifest {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "template.json"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := manifest.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestRunWritesThenChecks(t *testing.T) {
	t.Parallel()
	src := newUpstream().serve(t)
	path := filepath.Join(t.TempDir(), "manifest.json")
	tmpl, err := os.ReadFile(filepath.Join("testdata", "template.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, tmpl, 0o644); err != nil {
		t.Fatal(err)
	}
	check := []string{"-manifest", path, "-check"}
	if err := run(context.Background(), src, append(check, "-tag", "r0002", "-revision", "main")); err == nil {
		t.Fatal("check against a stale manifest passed")
	}
	if err := run(context.Background(), src, []string{"-manifest", path, "-tag", "r0002", "-revision", "main"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := run(context.Background(), src, check); err != nil {
		t.Fatalf("check after write: %v", err)
	}
}
