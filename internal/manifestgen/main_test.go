package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
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

	noSums   bool   // release without a SHA256SUMS asset
	sumsPad  int    // bytes of padding appended to SHA256SUMS
	revSHA   string // commit the main branch resolves to
	pageSize int    // tree entries per page; 0 disables pagination
	loopLink bool   // tree pages link back to themselves

	mu   sync.Mutex
	auth map[string]string // request path -> Authorization header
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
		lfs:    map[string]bool{"multilingual/model.safetensors": true, "english/model.safetensors": true},
		revSHA: newRev,
		auth:   map[string]string{},
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
	// The GitHub API lives under /gh so tests can tell it apart from the
	// download and Hugging Face hosts, as the real hosts differ.
	mux.HandleFunc("GET /gh/repos/up/engine/releases/tags/r0002", func(w http.ResponseWriter, _ *http.Request) {
		var rel ghRelease
		for name, body := range u.assets {
			d := u.digest[name]
			if d != "" {
				d = "sha256:" + d
			}
			rel.Assets = append(rel.Assets, ghAsset{name, int64(len(body)), d, srv.URL + "/dl/" + name})
		}
		if !u.noSums {
			rel.Assets = append(rel.Assets, ghAsset{Name: "SHA256SUMS", URL: srv.URL + "/dl/SHA256SUMS"})
		}
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
		_, _ = w.Write(bytes.Repeat([]byte("\n"), u.sumsPad))
	})
	mux.HandleFunc("GET /api/models/up/model/revision/main", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"sha":"` + u.revSHA + `"}`))
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
		sort.Slice(tree, func(i, j int) bool { return tree[i].Path < tree[j].Path })
		if u.loopLink {
			w.Header().Set("Link", `<`+srv.URL+r.URL.RequestURI()+`>; rel="next"`)
		} else if u.pageSize > 0 {
			from, _ := strconv.Atoi(r.URL.Query().Get("cursor"))
			if to := from + u.pageSize; to < len(tree) {
				w.Header().Set("Link", `<`+srv.URL+r.URL.Path+`?recursive=true&cursor=`+strconv.Itoa(to)+`>; rel="next"`)
				tree = tree[:to]
			}
			tree = tree[from:]
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
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.mu.Lock()
		u.auth[r.URL.Path] = r.Header.Get("Authorization")
		u.mu.Unlock()
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return Sources{Client: srv.Client(), GitHubAPI: srv.URL + "/gh", HFBase: srv.URL}
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
	err = run(context.Background(), src, append(check, "-tag", "r0002", "-revision", "main"))
	if err == nil || !strings.Contains(err.Error(), "out of date") {
		t.Fatalf("check against a stale manifest: err = %v, want out of date", err)
	}
	if err := run(context.Background(), src, []string{"-manifest", path, "-tag", "r0002", "-revision", "main"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := run(context.Background(), src, check); err != nil {
		t.Fatalf("check after write: %v", err)
	}
}

func TestGenerateClearsSignatureOnlyWhenContentChanges(t *testing.T) {
	t.Parallel()
	src := newUpstream().serve(t)
	tmpl := loadTemplate(t)
	tmpl.Signature = "signed-old-content"
	m, err := Generate(context.Background(), src, tmpl, "r0002", newRev)
	if err != nil {
		t.Fatal(err)
	}
	if m.Signature != "" {
		t.Errorf("Signature = %q after content changed, want it cleared", m.Signature)
	}
	m.Signature = "signed-current-content"
	again, err := Generate(context.Background(), src, m, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if again.Signature != "signed-current-content" {
		t.Errorf("Signature = %q with unchanged content, want it kept", again.Signature)
	}
}

func TestTokenGoesOnlyToGitHubAPI(t *testing.T) {
	t.Parallel()
	u := newUpstream()
	src := u.serve(t)
	src.Token = "secret"
	if _, err := Generate(context.Background(), src, loadTemplate(t), "r0002", "main"); err != nil {
		t.Fatal(err)
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.auth) < 4 {
		t.Fatalf("recorded %d requests, want the release, SHA256SUMS and Hugging Face calls", len(u.auth))
	}
	for path, auth := range u.auth {
		if onAPI := strings.HasPrefix(path, "/gh/"); onAPI != (auth == "Bearer secret") {
			t.Errorf("%s: Authorization = %q", path, auth)
		}
	}
}
