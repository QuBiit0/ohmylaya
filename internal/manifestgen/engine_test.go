package main

import (
	"context"
	"strings"
	"testing"
)

func TestGenerateRefreshesEngineAssets(t *testing.T) {
	t.Parallel()
	src := newUpstream().serve(t)
	m, err := Generate(context.Background(), src, loadTemplate(t), "r0002", "")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if m.Engine.Tag != "r0002" {
		t.Errorf("Tag = %q, want r0002", m.Engine.Tag)
	}
	a, err := m.EngineAsset("windows", "amd64", "cuda")
	if err != nil {
		t.Fatal(err)
	}
	if a.Name != "laya-r0002-windows-amd64-cuda.exe" || a.SHA256 != sum("windows engine") || a.Size != 14 {
		t.Errorf("asset = %+v", a)
	}
	if a.URL != strings.TrimSuffix(src.GitHubAPI, "/gh")+"/dl/laya-r0002-windows-amd64-cuda.exe" {
		t.Errorf("URL = %q", a.URL)
	}
	if a.Extra == nil || a.Extra.Name != "cublas.zip" {
		t.Errorf("extra archive not carried over: %+v", a.Extra)
	}
}

func TestGenerateFailsOnInconsistentRelease(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		breakIt func(u *upstream)
		wantErr string
	}{
		"sums disagree with github": {func(u *upstream) { u.sums["laya-r0002-linux-amd64-vulkan"] = sum("tampered") }, "disagrees"},
		"asset missing":             {func(u *upstream) { delete(u.assets, "laya-r0002-linux-amd64-vulkan") }, "not in release"},
		"asset not in sums":         {func(u *upstream) { delete(u.sums, "laya-r0002-windows-amd64-cuda.exe") }, "SHA256SUMS"},
		"no github digest":          {func(u *upstream) { u.digest["laya-r0002-linux-amd64-vulkan"] = "" }, "no digest"},
		"no SHA256SUMS asset":       {func(u *upstream) { u.noSums = true }, "has no SHA256SUMS"},
		"oversized SHA256SUMS":      {func(u *upstream) { u.sumsPad = maxBody }, "exceeds"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			u := newUpstream()
			tc.breakIt(u)
			_, err := Generate(context.Background(), u.serve(t), loadTemplate(t), "r0002", "")
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestGenerateFailsOnUnknownTag(t *testing.T) {
	t.Parallel()
	_, err := Generate(context.Background(), newUpstream().serve(t), loadTemplate(t), "r9999", "")
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("err = %v, want a 404", err)
	}
}
