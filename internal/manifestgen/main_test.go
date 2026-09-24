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
	"testing"

	"github.com/QuBiit0/ohmylaya/internal/manifest"
)

func sum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// upstream fakes a GitHub release. Tests break
// one of its maps to simulate an inconsistent upstream.
type upstream struct {
	assets map[string]string // release asset name -> content
	sums   map[string]string // SHA256SUMS entries, name -> digest
	digest map[string]string // GitHub API digest, name -> digest
}

func newUpstream() *upstream {
	u := &upstream{
		assets: map[string]string{
			"laya-r0002-linux-amd64-vulkan":     "linux engine",
			"laya-r0002-windows-amd64-cuda.exe": "windows engine",
		},
		sums: map[string]string{}, digest: map[string]string{},
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
	if err := run(context.Background(), src, append(check, "-tag", "r0002")); err == nil {
		t.Fatal("check against a stale manifest passed")
	}
	if err := run(context.Background(), src, []string{"-manifest", path, "-tag", "r0002"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := run(context.Background(), src, check); err != nil {
		t.Fatalf("check after write: %v", err)
	}
}
