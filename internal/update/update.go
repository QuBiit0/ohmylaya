package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ErrUpToDate is returned when no newer release exists.
var ErrUpToDate = errors.New("update: already up to date")

// Plan is a resolved update.
type Plan struct {
	Current string
	Latest  string
	Asset   Asset
	SHA256  string
}

// ArchiveName follows the GoReleaser template in .goreleaser.yaml.
func ArchiveName(version, goos, goarch string) string {
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("ohmylaya_%s_%s_%s.%s", strings.TrimPrefix(version, "v"), goos, goarch, ext)
}

// Check resolves the latest release and its checksum for this platform.
func Check(ctx context.Context, client *http.Client, current string) (*Plan, error) {
	rel, err := FetchLatest(ctx, client)
	if err != nil {
		return nil, err
	}
	p := &Plan{Current: current, Latest: rel.TagName}
	if current != "dev" && strings.TrimPrefix(current, "v") == strings.TrimPrefix(rel.TagName, "v") {
		return p, ErrUpToDate
	}
	name := ArchiveName(rel.TagName, runtime.GOOS, runtime.GOARCH)
	var sums Asset
	for _, a := range rel.Assets {
		switch a.Name {
		case name:
			p.Asset = a
		case "checksums.txt":
			sums = a
		}
	}
	if p.Asset.URL == "" || sums.URL == "" {
		return nil, fmt.Errorf("update: release %s has no %s or checksums.txt", rel.TagName, name)
	}
	body, err := get(ctx, client, sums.URL)
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == name {
			p.SHA256 = fields[0]
		}
	}
	if p.SHA256 == "" {
		return nil, fmt.Errorf("update: checksums.txt has no entry for %s", name)
	}
	return p, nil
}

// Apply downloads the archive, verifies it and swaps binPath. On Windows
// the running executable cannot be replaced, so the new binary is placed
// beside it as binPath+".new" and swapped by SwapPending on next start.
func Apply(ctx context.Context, client *http.Client, p *Plan, binPath string) error {
	archive, err := get(ctx, client, p.Asset.URL)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(archive)
	if got := hex.EncodeToString(sum[:]); !strings.EqualFold(got, p.SHA256) {
		return fmt.Errorf("update: checksum mismatch for %s: expected %s, got %s", p.Asset.Name, p.SHA256, got)
	}
	exe, err := extractBinary(archive, p.Asset.Name)
	if err != nil {
		return err
	}
	return swap(binPath, exe)
}

func swap(binPath string, exe []byte) error {
	tmp := binPath + ".new"
	if err := os.WriteFile(tmp, exe, 0o755); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		// Rename the running binary aside, then move the new one in.
		old := binPath + ".old"
		_ = os.Remove(old)
		if err := os.Rename(binPath, old); err != nil {
			return fmt.Errorf("update: staged at %s; restart to apply: %w", tmp, err)
		}
		if err := os.Rename(tmp, binPath); err != nil {
			_ = os.Rename(old, binPath)
			return err
		}
		return nil
	}
	return os.Rename(tmp, binPath)
}

// SwapPending finishes a Windows update on start: removes a leftover .old
// and applies a staged .new when the previous swap could not complete.
func SwapPending(binPath string) {
	_ = os.Remove(binPath + ".old")
	if _, err := os.Stat(binPath + ".new"); err == nil {
		if err := os.Rename(binPath, binPath+".old"); err == nil {
			if os.Rename(binPath+".new", binPath) != nil {
				_ = os.Rename(binPath+".old", binPath)
			}
		}
	}
}

func extractBinary(archive []byte, name string) ([]byte, error) {
	want := "ohmylaya"
	if strings.HasSuffix(name, ".zip") {
		want += ".exe"
		zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return nil, err
		}
		for _, f := range zr.File {
			if filepath.Base(f.Name) == want {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return io.ReadAll(rc)
			}
		}
		return nil, fmt.Errorf("update: %s not found in %s", want, name)
	}
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if filepath.Base(h.Name) == want && h.Typeflag == tar.TypeReg {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("update: %s not found in %s", want, name)
}

func get(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ohmylaya")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update: %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}
