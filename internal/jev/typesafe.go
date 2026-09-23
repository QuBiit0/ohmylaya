package jev

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// DefaultTypeSafeURL is the hosted Jev endpoint base.
const DefaultTypeSafeURL = "https://api.typesafe.ai"

// TypeSafe talks to the hosted Jev API, one request per call.
type TypeSafe struct {
	baseURL string
	apiKey  string
	client  *http.Client
	backoff func(int)
}

// NewTypeSafe creates the hosted provider. baseURL may be empty for the
// public endpoint.
func NewTypeSafe(baseURL, apiKey string, client *http.Client) *TypeSafe {
	if baseURL == "" {
		baseURL = DefaultTypeSafeURL
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &TypeSafe{baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, client: client, backoff: defaultBackoff}
}

// Name returns "typesafe".
func (t *TypeSafe) Name() string { return "typesafe" }

// Predict posts each request to /v1/systemone with a bearer token, retrying
// on 429 and 529 with backoff.
func (t *TypeSafe) Predict(ctx context.Context, reqs []Request) ([]Response, error) {
	out := make([]Response, 0, len(reqs))
	headers := map[string]string{"Authorization": "Bearer " + t.apiKey}
	retry := map[int]bool{http.StatusTooManyRequests: true, 529: true}
	for _, r := range reqs {
		r.ID = ""
		if r.Model == "" {
			r.Model = "jev-latest"
		}
		data, err := post(ctx, t.client, t.baseURL+"/v1/systemone", headers, r, retry, t.backoff)
		if err != nil {
			return nil, err
		}
		var resp Response
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, fmt.Errorf("jev: decode typesafe response: %w", err)
		}
		resp.Provider = t.Name()
		out = append(out, resp)
	}
	return out, nil
}
