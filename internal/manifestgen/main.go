// Command manifestgen regenerates internal/manifest/manifest.json from the
// upstream laya.cpp release. It runs at ohmylaya release time and is not
// shipped.
//
//	go run ./internal/manifestgen [-tag r0003] [-check]
//
// The current manifest is the template: it decides which platforms and
// backends are published, the auxiliary archives, and the model files, which
// are carried over unchanged. Engine names, URLs, sizes and digests are
// refreshed from upstream. The run fails when upstream contradicts itself: a
// SHA256SUMS entry that differs from the digest GitHub computed for the asset.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/QuBiit0/ohmylaya/internal/manifest"
)

// Sources are the upstream endpoints, overridable in tests.
type Sources struct {
	Client    *http.Client
	GitHubAPI string
	HFBase    string
	Token     string // optional, sent only to GitHubAPI to raise the rate limit
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	src := Sources{
		Client:    &http.Client{Timeout: time.Minute},
		GitHubAPI: "https://api.github.com",
		HFBase:    "https://huggingface.co",
		Token:     os.Getenv("GITHUB_TOKEN"),
	}
	if err := run(ctx, src, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "manifestgen:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, src Sources, args []string) error {
	fs := flag.NewFlagSet("manifestgen", flag.ContinueOnError)
	path := fs.String("manifest", filepath.Join("internal", "manifest", "manifest.json"), "manifest used as template and written back")
	tag := fs.String("tag", "", "laya.cpp release tag (default: the manifest's)")
	check := fs.Bool("check", false, "fail when the manifest differs from upstream instead of writing it")
	if err := fs.Parse(args); err != nil {
		return err
	}
	b, err := os.ReadFile(*path)
	if err != nil {
		return err
	}
	tmpl, err := manifest.Parse(b)
	if err != nil {
		return err
	}
	m, err := Generate(ctx, src, tmpl, *tag)
	if err != nil {
		return err
	}
	if *check {
		if !reflect.DeepEqual(tmpl, m) {
			return fmt.Errorf("%s is out of date with upstream; rerun without -check", *path)
		}
		return nil
	}
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(*path, append(out, '\n'), 0o644)
}

// Generate returns a copy of tmpl refreshed from upstream at the given tag; an
// empty tag keeps the template's.
func Generate(ctx context.Context, src Sources, tmpl *manifest.Manifest, tag string) (*manifest.Manifest, error) {
	b, err := json.Marshal(tmpl)
	if err != nil {
		return nil, err
	}
	var m manifest.Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	oldTag := m.Engine.Tag
	if tag != "" {
		m.Engine.Tag = tag
	}
	if err := refreshEngine(ctx, src, &m, oldTag); err != nil {
		return nil, err
	}
	return &m, m.Validate()
}

// maxBody bounds everything manifestgen reads into memory.
const maxBody = 16 << 20

func getJSON(ctx context.Context, src Sources, url string, v any) error {
	b, err := get(ctx, src, url)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", url, err)
	}
	return nil
}

func get(ctx context.Context, src Sources, url string) ([]byte, error) {
	resp, err := do(ctx, src, url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, maxBody))
}

func do(ctx context.Context, src Sources, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if src.Token != "" && strings.HasPrefix(url, src.GitHubAPI+"/") {
		req.Header.Set("Authorization", "Bearer "+src.Token)
	}
	resp, err := src.Client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return resp, nil
}
