package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/QuBiit0/ohmylaya/internal/manifest"
)

type ghRelease struct {
	Assets []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
	URL    string `json:"browser_download_url"`
}

func refreshEngine(ctx context.Context, src Sources, m *manifest.Manifest, oldTag string) error {
	var rel ghRelease
	if err := getJSON(ctx, src, src.GitHubAPI+"/repos/"+m.Engine.Project+"/releases/tags/"+m.Engine.Tag, &rel); err != nil {
		return err
	}
	byName := map[string]ghAsset{}
	for _, a := range rel.Assets {
		byName[a.Name] = a
	}
	sumsAsset, ok := byName["SHA256SUMS"]
	if !ok {
		return fmt.Errorf("release %s has no SHA256SUMS", m.Engine.Tag)
	}
	body, err := get(ctx, src, sumsAsset.URL)
	if err != nil {
		return err
	}
	sums := parseSums(body)
	for i := range m.Engine.Assets {
		a := &m.Engine.Assets[i]
		name := strings.ReplaceAll(a.Name, oldTag, m.Engine.Tag)
		up, ok := byName[name]
		if !ok {
			return fmt.Errorf("%s is not in release %s", name, m.Engine.Tag)
		}
		want, ok := sums[name]
		if !ok {
			return fmt.Errorf("%s has no SHA256SUMS entry", name)
		}
		if up.Digest == "" {
			return fmt.Errorf("%s: GitHub reports no digest, so SHA256SUMS cannot be cross-checked", name)
		}
		if got := strings.TrimPrefix(up.Digest, "sha256:"); got != want {
			return fmt.Errorf("%s: SHA256SUMS %s disagrees with the GitHub digest %q", name, want, got)
		}
		a.Name, a.URL, a.Size, a.SHA256 = name, up.URL, up.Size, want
	}
	return nil
}

// parseSums reads "<sha256>  <name>" lines; a leading "*" marks binary mode.
func parseSums(b []byte) map[string]string {
	sums := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		if f := strings.Fields(sc.Text()); len(f) == 2 {
			sums[strings.TrimPrefix(f[1], "*")] = strings.ToLower(f[0])
		}
	}
	return sums
}
