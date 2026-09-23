// Package update checks GitHub releases and replaces the running binary.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Repo is the GitHub repository that publishes releases.
const Repo = "QuBiit0/ohmylaya"

// Release is the subset of the GitHub release document ohmylaya uses.
type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

// Asset is one downloadable release file.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

// APIBase is overridable for tests.
var APIBase = "https://api.github.com"

// FetchLatest returns the latest non-prerelease release.
func FetchLatest(ctx context.Context, client *http.Client) (*Release, error) {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, APIBase+"/repos/"+Repo+"/releases/latest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ohmylaya")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update: GitHub returned %s", resp.Status)
	}
	var r Release
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	return &r, nil
}

// LatestRelease returns only the tag, for the doctor.
func LatestRelease(ctx context.Context) (string, error) {
	r, err := FetchLatest(ctx, nil)
	if err != nil {
		return "", err
	}
	return r.TagName, nil
}
