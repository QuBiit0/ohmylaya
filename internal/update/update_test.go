package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func archive(t *testing.T, name string, exe []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	if strings.HasSuffix(name, ".zip") {
		zw := zip.NewWriter(&buf)
		w, _ := zw.Create("ohmylaya.exe")
		w.Write(exe)
		zw.Close()
		return buf.Bytes()
	}
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "ohmylaya", Mode: 0o755, Size: int64(len(exe)), Typeflag: tar.TypeReg})
	tw.Write(exe)
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func server(t *testing.T, tag string, exe []byte) *httptest.Server {
	t.Helper()
	name := ArchiveName(tag, runtime.GOOS, runtime.GOARCH)
	arch := archive(t, name, exe)
	sum := sha256.Sum256(arch)
	sums := hex.EncodeToString(sum[:]) + "  " + name + "\nabc  other.tar.gz\n"
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			json.NewEncoder(w).Encode(Release{TagName: tag, Assets: []Asset{
				{Name: name, URL: srv.URL + "/dl/" + name, Size: int64(len(arch))},
				{Name: "checksums.txt", URL: srv.URL + "/dl/checksums.txt"},
			}})
		case r.URL.Path == "/dl/"+name:
			w.Write(arch)
		case r.URL.Path == "/dl/checksums.txt":
			w.Write([]byte(sums))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCheckAndApplyReplacesBinary(t *testing.T) {
	exe := []byte("new binary contents")
	srv := server(t, "v2.0.0", exe)
	old := APIBase
	APIBase = srv.URL
	t.Cleanup(func() { APIBase = old })

	bin := filepath.Join(t.TempDir(), "ohmylaya")
	os.WriteFile(bin, []byte("old"), 0o755)

	p, err := Check(context.Background(), srv.Client(), "v1.0.0")
	if err != nil {
		t.Fatalf("Check() = %v", err)
	}
	if p.Latest != "v2.0.0" || p.SHA256 == "" {
		t.Errorf("plan = %+v", p)
	}
	if err := Apply(context.Background(), srv.Client(), p, bin); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	got, _ := os.ReadFile(bin)
	if !bytes.Equal(got, exe) {
		t.Errorf("binary = %q", got)
	}
	if runtime.GOOS == "windows" {
		if _, err := os.Stat(bin + ".old"); err != nil {
			t.Error("windows swap should leave .old for cleanup")
		}
		SwapPending(bin)
		if _, err := os.Stat(bin + ".old"); !errors.Is(err, os.ErrNotExist) {
			t.Error("SwapPending must remove .old")
		}
	}
}

func TestCheckUpToDate(t *testing.T) {
	srv := server(t, "v1.0.0", []byte("x"))
	old := APIBase
	APIBase = srv.URL
	t.Cleanup(func() { APIBase = old })
	_, err := Check(context.Background(), srv.Client(), "1.0.0")
	if !errors.Is(err, ErrUpToDate) {
		t.Errorf("err = %v, want ErrUpToDate", err)
	}
	if _, err := Check(context.Background(), srv.Client(), "dev"); err != nil {
		t.Errorf("dev builds always see an update: %v", err)
	}
}

func TestApplyRejectsChecksumMismatch(t *testing.T) {
	srv := server(t, "v2.0.0", []byte("exe"))
	old := APIBase
	APIBase = srv.URL
	t.Cleanup(func() { APIBase = old })
	p, err := Check(context.Background(), srv.Client(), "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	p.SHA256 = strings.Repeat("0", 64)
	bin := filepath.Join(t.TempDir(), "ohmylaya")
	os.WriteFile(bin, []byte("old"), 0o755)
	if err := Apply(context.Background(), srv.Client(), p, bin); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("err = %v", err)
	}
	if got, _ := os.ReadFile(bin); string(got) != "old" {
		t.Error("binary must be untouched after a mismatch")
	}
}

func TestSwapPendingAppliesStagedBinary(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "ohmylaya")
	os.WriteFile(bin, []byte("old"), 0o755)
	os.WriteFile(bin+".new", []byte("staged"), 0o755)
	SwapPending(bin)
	got, _ := os.ReadFile(bin)
	if string(got) != "staged" {
		t.Errorf("binary = %q, want staged", got)
	}
}
