package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Provider answers JEV requests.
type Provider interface {
	// Predict evaluates every request and returns one response per request
	// in the same order.
	Predict(ctx context.Context, reqs []Request) ([]Response, error)
	// Name identifies the provider in tool results.
	Name() string
}

// Sentinel errors.
var (
	ErrOverloaded     = errors.New("jev: provider overloaded")
	ErrInvalidRequest = errors.New("jev: invalid request")
	ErrUnauthorized   = errors.New("jev: unauthorized")
)

const maxAttempts = 3

// errorBody is the JSON error envelope both laya.cpp and TypeSafe use.
type errorBody struct {
	Error struct {
		Message string `json:"message"`
		Status  int    `json:"status"`
	} `json:"error"`
}

func defaultBackoff(attempt int) {
	time.Sleep(time.Duration(attempt*attempt) * 250 * time.Millisecond)
}

// post sends a JSON body and returns the response body, retrying on the
// status codes in retryOn.
func post(ctx context.Context, client *http.Client, url string, headers map[string]string, body any, retryOn map[int]bool, backoff func(int)) ([]byte, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("jev: encode request: %w", err)
	}
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("jev: %s: %w", url, err)
		}
		data, rerr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if rerr != nil {
			return nil, fmt.Errorf("jev: read response: %w", rerr)
		}
		switch {
		case resp.StatusCode == http.StatusOK:
			return data, nil
		case retryOn[resp.StatusCode]:
			lastErr = fmt.Errorf("%w: %s (%s)", ErrOverloaded, resp.Status, engineMessage(data))
			if attempt < maxAttempts {
				backoff(attempt)
			}
			continue
		case resp.StatusCode == http.StatusUnprocessableEntity || resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusRequestEntityTooLarge:
			return nil, fmt.Errorf("%w: %s", ErrInvalidRequest, engineMessage(data))
		case resp.StatusCode == http.StatusUnauthorized:
			return nil, fmt.Errorf("%w: %s", ErrUnauthorized, engineMessage(data))
		default:
			return nil, fmt.Errorf("jev: %s: %s", resp.Status, engineMessage(data))
		}
	}
	return nil, lastErr
}

func engineMessage(data []byte) string {
	var eb errorBody
	if json.Unmarshal(data, &eb) == nil && eb.Error.Message != "" {
		return eb.Error.Message
	}
	s := string(bytes.TrimSpace(data))
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}
