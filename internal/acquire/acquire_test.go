package acquire

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func payload(t *testing.T, n int) ([]byte, string) {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	return b, hex.EncodeToString(sum[:])
}

// rangeServer serves body with Range support. cutAfter > 0 closes the
// connection after that many bytes on the first request only, to simulate an
// interrupted download.
func rangeServer(t *testing.T, body []byte, cutAfter int, supportRange bool) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	h := func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		start := 0
		if rng := r.Header.Get("Range"); rng != "" && supportRange {
			fmt.Sscanf(rng, "bytes=%d-", &start)
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(body)-1, len(body)))
			w.Header().Set("Content-Length", strconv.Itoa(len(body)-start))
			w.WriteHeader(http.StatusPartialContent)
		} else {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			w.WriteHeader(http.StatusOK)
		}
		chunk := body[start:]
		if n == 1 && cutAfter > 0 && cutAfter < len(chunk) {
			w.Write(chunk[:cutAfter])
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			// Abort the connection so the client sees an unexpected EOF.
			panic(http.ErrAbortHandler)
		}
		w.Write(chunk)
	}
	srv := httptest.NewServer(http.HandlerFunc(h))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestDownloadVerifiesAndPlacesAtomically(t *testing.T) {
	t.Parallel()
	body, sum := payload(t, 64*1024+7)
	srv, _ := rangeServer(t, body, 0, true)
	dest := filepath.Join(t.TempDir(), "asset.bin")

	var last int64
	err := Download(context.Background(), srv.Client(), Spec{URL: srv.URL, Size: int64(len(body)), SHA256: sum, Dest: dest},
		func(done, total int64) { last = done })
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	got, _ := os.ReadFile(dest)
	if !bytes.Equal(got, body) {
		t.Fatal("placed file differs from payload")
	}
	if last != int64(len(body)) {
		t.Errorf("last progress = %d, want %d", last, len(body))
	}
	if _, err := os.Stat(dest + PartSuffix); !errors.Is(err, os.ErrNotExist) {
		t.Error("part file should be gone after success")
	}
}

func TestDownloadResumesAfterInterruption(t *testing.T) {
	t.Parallel()
	body, sum := payload(t, 200*1024)
	srv, calls := rangeServer(t, body, 50*1024, true)
	dest := filepath.Join(t.TempDir(), "asset.bin")
	spec := Spec{URL: srv.URL, Size: int64(len(body)), SHA256: sum, Dest: dest}

	err := Download(context.Background(), srv.Client(), spec, nil)
	if err == nil {
		t.Fatal("first attempt should fail on the cut connection")
	}
	part, err := os.Stat(dest + PartSuffix)
	if err != nil {
		t.Fatalf("part file missing after interruption: %v", err)
	}
	if part.Size() == 0 || part.Size() >= int64(len(body)) {
		t.Fatalf("part size = %d, want partial", part.Size())
	}

	if err := Download(context.Background(), srv.Client(), spec, nil); err != nil {
		t.Fatalf("resume error = %v", err)
	}
	got, _ := os.ReadFile(dest)
	if !bytes.Equal(got, body) {
		t.Fatal("resumed file differs from payload")
	}
	if calls.Load() != 2 {
		t.Errorf("server calls = %d, want 2", calls.Load())
	}
}

func TestDownloadRestartsWhenServerIgnoresRange(t *testing.T) {
	t.Parallel()
	body, sum := payload(t, 100*1024)
	srv, _ := rangeServer(t, body, 0, false)
	dest := filepath.Join(t.TempDir(), "asset.bin")
	// Pre-seed a bogus partial file.
	if err := os.WriteFile(dest+PartSuffix, []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := Spec{URL: srv.URL, Size: int64(len(body)), SHA256: sum, Dest: dest}
	if err := Download(context.Background(), srv.Client(), spec, nil); err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	got, _ := os.ReadFile(dest)
	if !bytes.Equal(got, body) {
		t.Fatal("file differs from payload after restart")
	}
}

func TestDownloadRejectsDigestMismatchAndLeavesNoFinalFile(t *testing.T) {
	t.Parallel()
	body, _ := payload(t, 8*1024)
	srv, _ := rangeServer(t, body, 0, true)
	dest := filepath.Join(t.TempDir(), "asset.bin")
	spec := Spec{URL: srv.URL, Size: int64(len(body)), SHA256: strings.Repeat("0", 64), Dest: dest}

	err := Download(context.Background(), srv.Client(), spec, nil)
	if !errors.Is(err, ErrDigest) {
		t.Fatalf("err = %v, want ErrDigest", err)
	}
	if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
		t.Error("final file must not exist after a digest mismatch")
	}
	if _, err := os.Stat(dest + PartSuffix); !errors.Is(err, os.ErrNotExist) {
		t.Error("part file must be deleted after a digest mismatch")
	}
}

func TestDownloadSkipsWhenDestAlreadyMatches(t *testing.T) {
	t.Parallel()
	body, sum := payload(t, 4*1024)
	srv, calls := rangeServer(t, body, 0, true)
	dest := filepath.Join(t.TempDir(), "asset.bin")
	if err := os.WriteFile(dest, body, 0o644); err != nil {
		t.Fatal(err)
	}
	spec := Spec{URL: srv.URL, Size: int64(len(body)), SHA256: sum, Dest: dest}
	if err := Download(context.Background(), srv.Client(), spec, nil); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Errorf("server calls = %d, want 0 when dest already matches", calls.Load())
	}
}

func TestDownloadHonoursContext(t *testing.T) {
	t.Parallel()
	body, sum := payload(t, 1024)
	srv, _ := rangeServer(t, body, 0, true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Download(ctx, srv.Client(), Spec{URL: srv.URL, Size: 1024, SHA256: sum, Dest: filepath.Join(t.TempDir(), "x")}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestVerifyFile(t *testing.T) {
	t.Parallel()
	body, sum := payload(t, 3000)
	p := filepath.Join(t.TempDir(), "f")
	os.WriteFile(p, body, 0o644)
	if err := VerifyFile(p, int64(len(body)), sum); err != nil {
		t.Errorf("VerifyFile() = %v, want nil", err)
	}
	if err := VerifyFile(p, int64(len(body))+1, sum); !errors.Is(err, ErrSize) {
		t.Errorf("wrong size err = %v, want ErrSize", err)
	}
	if err := VerifyFile(p, int64(len(body)), strings.Repeat("f", 64)); !errors.Is(err, ErrDigest) {
		t.Errorf("wrong digest err = %v, want ErrDigest", err)
	}
	if err := VerifyFile(filepath.Join(t.TempDir(), "missing"), 1, sum); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing err = %v, want ErrNotExist", err)
	}
}

func TestExtractMembers(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range map[string]string{
		"libcublas/bin/cublas64_13.dll":   "dll-a",
		"libcublas/bin/cublasLt64_13.dll": "dll-b",
		"libcublas/LICENSE":               "lic",
		"libcublas/bin/other.dll":         "ignored",
	} {
		w, _ := zw.Create(name)
		w.Write([]byte(content))
	}
	zw.Close()
	zipPath := filepath.Join(t.TempDir(), "a.zip")
	os.WriteFile(zipPath, buf.Bytes(), 0o644)

	dest := t.TempDir()
	err := ExtractMembers(zipPath, []string{"bin/cublas64_13.dll", "bin/cublasLt64_13.dll", "LICENSE"}, dest)
	if err != nil {
		t.Fatalf("ExtractMembers() = %v", err)
	}
	for name, want := range map[string]string{"cublas64_13.dll": "dll-a", "cublasLt64_13.dll": "dll-b", "LICENSE": "lic"} {
		got, err := os.ReadFile(filepath.Join(dest, name))
		if err != nil || string(got) != want {
			t.Errorf("%s = %q, %v; want %q", name, got, err, want)
		}
	}
	if _, err := os.Stat(filepath.Join(dest, "other.dll")); !errors.Is(err, os.ErrNotExist) {
		t.Error("unlisted member must not be extracted")
	}
	if err := ExtractMembers(zipPath, []string{"bin/missing.dll"}, dest); err == nil {
		t.Error("missing member must be an error")
	}
}
