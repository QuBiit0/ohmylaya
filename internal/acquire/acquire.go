// Package acquire downloads engine and model files with resume support,
// verifies them against pinned digests, and places them atomically.
package acquire

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// PartSuffix is appended to the destination while a download is in progress.
const PartSuffix = ".part"

// Sentinel errors.
var (
	ErrDigest = errors.New("acquire: sha256 mismatch")
	ErrSize   = errors.New("acquire: size mismatch")
)

// Spec describes one file to fetch.
type Spec struct {
	URL    string
	Size   int64
	SHA256 string
	Dest   string
}

// Progress receives the bytes done so far and the expected total.
type Progress func(done, total int64)

// Download fetches spec.URL into spec.Dest. A previous partial download at
// Dest+PartSuffix is resumed with a Range request when the server supports
// it. The file is only visible at Dest after its size and SHA-256 matched.
// When Dest already exists and matches, nothing is downloaded.
func Download(ctx context.Context, client *http.Client, spec Spec, progress Progress) error {
	if client == nil {
		client = http.DefaultClient
	}
	if progress == nil {
		progress = func(int64, int64) {}
	}
	if err := VerifyFile(spec.Dest, spec.Size, spec.SHA256); err == nil {
		progress(spec.Size, spec.Size)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(spec.Dest), 0o755); err != nil {
		return fmt.Errorf("acquire: create directory: %w", err)
	}

	part := spec.Dest + PartSuffix
	if err := fetch(ctx, client, spec, part, progress); err != nil {
		return err
	}
	if err := VerifyFile(part, spec.Size, spec.SHA256); err != nil {
		_ = os.Remove(part)
		return fmt.Errorf("%w (%s)", err, path.Base(spec.URL))
	}
	if err := os.Rename(part, spec.Dest); err != nil {
		return fmt.Errorf("acquire: place %s: %w", spec.Dest, err)
	}
	return nil
}

// fetch streams the body into part, resuming when possible.
func fetch(ctx context.Context, client *http.Client, spec Spec, part string, progress Progress) error {
	var offset int64
	if st, err := os.Stat(part); err == nil && st.Size() > 0 && st.Size() < spec.Size {
		offset = st.Size()
	} else if err == nil {
		// Empty or oversized partial: start over.
		_ = os.Remove(part)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, spec.URL, nil)
	if err != nil {
		return fmt.Errorf("acquire: build request: %w", err)
	}
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("acquire: %s: %w", path.Base(spec.URL), err)
	}
	defer resp.Body.Close()

	flags := os.O_CREATE | os.O_WRONLY
	switch resp.StatusCode {
	case http.StatusPartialContent:
		flags |= os.O_APPEND
	case http.StatusOK:
		// Server ignored the range or we started fresh: rewrite from zero.
		offset = 0
		flags |= os.O_TRUNC
	default:
		return fmt.Errorf("acquire: %s: unexpected status %s", path.Base(spec.URL), resp.Status)
	}

	f, err := os.OpenFile(part, flags, 0o644)
	if err != nil {
		return fmt.Errorf("acquire: open partial: %w", err)
	}
	defer f.Close()

	done := offset
	progress(done, spec.Size)
	buf := make([]byte, 256*1024)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return fmt.Errorf("acquire: write partial: %w", werr)
			}
			done += int64(n)
			progress(done, spec.Size)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("acquire: %s: interrupted after %d bytes: %w", path.Base(spec.URL), done, rerr)
		}
	}
	return f.Sync()
}

// VerifyFile checks that p exists with the expected size and SHA-256.
func VerifyFile(p string, size int64, sum string) error {
	st, err := os.Stat(p)
	if err != nil {
		return err
	}
	if st.Size() != size {
		return fmt.Errorf("%w: %s is %d bytes, want %d", ErrSize, filepath.Base(p), st.Size(), size)
	}
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, sum) {
		return fmt.Errorf("%w: %s got %s, want %s", ErrDigest, filepath.Base(p), got, sum)
	}
	return nil
}

// ExtractMembers copies the listed members out of a zip archive into
// destDir, flattening their paths. Members are matched by suffix so the
// archive's top-level directory name does not matter. Every listed member
// must exist.
func ExtractMembers(zipPath string, members []string, destDir string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("acquire: open %s: %w", filepath.Base(zipPath), err)
	}
	defer zr.Close()
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	for _, want := range members {
		var found *zip.File
		for _, f := range zr.File {
			if f.FileInfo().IsDir() {
				continue
			}
			if f.Name == want || strings.HasSuffix(f.Name, "/"+want) {
				found = f
				break
			}
		}
		if found == nil {
			return fmt.Errorf("acquire: member %q not found in %s", want, filepath.Base(zipPath))
		}
		if err := extractOne(found, filepath.Join(destDir, path.Base(want))); err != nil {
			return err
		}
	}
	return nil
}

func extractOne(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	tmp := dest + PartSuffix
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}
