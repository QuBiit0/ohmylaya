package main

import (
	"context"
	"strings"
	"testing"

	"github.com/QuBiit0/ohmylaya/internal/manifest"
)

func TestGenerateRefreshesModelFiles(t *testing.T) {
	t.Parallel()
	m, err := Generate(context.Background(), newUpstream().serve(t), loadTemplate(t), "r0002", "main")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if m.Models.Revision != newRev {
		t.Errorf("Revision = %q, want the commit main points at, %s", m.Models.Revision, newRev)
	}
	v, _ := m.Variant("multilingual")
	want := []manifest.File{
		{Path: "model.safetensors", Size: 7, SHA256: sum("weights")},
		{Path: "encoder/config.json", Size: 12, SHA256: sum(`{"hidden":8}`)},
	}
	if v.Context != 1024 || v.Prefix != "multilingual/" || len(v.Files) != 2 || v.Files[0] != want[0] || v.Files[1] != want[1] {
		t.Errorf("variant = %+v, want files %+v", v, want)
	}
}

func TestGenerateFailsOnInconsistentRepository(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		breakIt func(u *upstream)
		wantErr string
	}{
		"model file missing":   {func(u *upstream) { delete(u.files, "multilingual/encoder/config.json") }, "not in repository"},
		"small file corrupted": {func(u *upstream) { u.oid["multilingual/encoder/config.json"] = strings.Repeat("0", 40) }, "blob oid"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			u := newUpstream()
			tc.breakIt(u)
			_, err := Generate(context.Background(), u.serve(t), loadTemplate(t), "r0002", newRev)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}
