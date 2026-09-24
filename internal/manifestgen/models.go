package main

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/QuBiit0/ohmylaya/internal/manifest"
)

type hfEntry struct {
	Type string `json:"type"`
	Path string `json:"path"`
	Size int64  `json:"size"`
	OID  string `json:"oid"`
	LFS  *hfLFS `json:"lfs,omitempty"`
}

type hfLFS struct {
	OID  string `json:"oid"`
	Size int64  `json:"size"`
}

var commitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// resolveRevision pins a branch or tag to its commit so the manifest never
// follows a moving ref.
func resolveRevision(ctx context.Context, src Sources, repo, rev string) (string, error) {
	if commitPattern.MatchString(rev) {
		return rev, nil
	}
	var info struct {
		SHA string `json:"sha"`
	}
	if err := getJSON(ctx, src, src.HFBase+"/api/models/"+repo+"/revision/"+rev, &info); err != nil {
		return "", err
	}
	if !commitPattern.MatchString(info.SHA) {
		return "", fmt.Errorf("revision %s of %s resolved to %q", rev, repo, info.SHA)
	}
	return info.SHA, nil
}

func refreshModels(ctx context.Context, src Sources, m *manifest.Manifest) error {
	repo, rev := m.Models.Repo, m.Models.Revision
	for _, v := range m.Models.Variants {
		url := src.HFBase + "/api/models/" + repo + "/tree/" + rev
		if p := strings.TrimSuffix(v.Prefix, "/"); p != "" {
			url += "/" + p
		}
		tree, err := listTree(ctx, src, url+"?recursive=true")
		if err != nil {
			return err
		}
		for i := range v.Files {
			f := &v.Files[i]
			full := v.Prefix + f.Path
			e, ok := tree[full]
			if !ok || e.Type != "file" {
				return fmt.Errorf("%s is not in repository %s at %s", full, repo, rev)
			}
			if e.LFS != nil {
				f.Size, f.SHA256 = e.LFS.Size, e.LFS.OID
				continue
			}
			body, err := get(ctx, src, src.HFBase+"/"+repo+"/resolve/"+rev+"/"+full)
			if err != nil {
				return err
			}
			if got := blobOID(body); got != e.OID {
				return fmt.Errorf("%s: content hashes to %s, not the git blob oid %s", full, got, e.OID)
			}
			digest := sha256.Sum256(body)
			f.Size, f.SHA256 = int64(len(body)), hex.EncodeToString(digest[:])
		}
	}
	return nil
}

var nextLink = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

// listTree follows the Hugging Face tree API's Link pagination.
func listTree(ctx context.Context, src Sources, url string) (map[string]hfEntry, error) {
	tree := map[string]hfEntry{}
	for url != "" {
		resp, err := do(ctx, src, url)
		if err != nil {
			return nil, err
		}
		var page []hfEntry
		err = json.NewDecoder(resp.Body).Decode(&page)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", url, err)
		}
		for _, e := range page {
			tree[e.Path] = e
		}
		url = ""
		if m := nextLink.FindStringSubmatch(resp.Header.Get("Link")); m != nil {
			url = m[1]
		}
	}
	return tree, nil
}

// blobOID is git's object id for a blob: sha1("blob <size>\x00<content>").
func blobOID(b []byte) string {
	h := sha1.New()
	h.Write([]byte("blob " + strconv.Itoa(len(b)) + "\x00"))
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}
