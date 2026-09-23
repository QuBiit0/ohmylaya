package jev

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// DefaultMaxBatchChars bounds the total state text per engine call. Eight
// full-length rows exhaust a 4 GB laptop GPU (68 s per call measured on an
// RTX 3050) while four take under a second, so calls are packed by size as
// well as by question count. About four rows of 1024 tokens at 3.2
// characters per token.
const DefaultMaxBatchChars = 12000

// Local talks to a laya.cpp HTTP server on loopback.
type Local struct {
	baseURL      string
	client       *http.Client
	maxQuestions int
	maxChars     int
	backoff      func(int)
}

// NewLocal creates a provider for the server at baseURL. maxQuestions is the
// engine's per-call question limit; larger requests are split and merged.
func NewLocal(baseURL string, client *http.Client, maxQuestions int) *Local {
	if client == nil {
		client = http.DefaultClient
	}
	if maxQuestions < 1 {
		maxQuestions = 8
	}
	return &Local{baseURL: strings.TrimRight(baseURL, "/"), client: client, maxQuestions: maxQuestions, maxChars: DefaultMaxBatchChars, backoff: defaultBackoff}
}

// SetMaxBatchChars overrides the per-call state size bound.
func (l *Local) SetMaxBatchChars(n int) {
	if n > 0 {
		l.maxChars = n
	}
}

// stateChars estimates the size of a request's state in characters.
func stateChars(r Request) int {
	switch s := r.State.(type) {
	case string:
		return len(s)
	default:
		b, _ := json.Marshal(s)
		return len(b)
	}
}

// Name returns "local".
func (l *Local) Name() string { return "local" }

// Health is the /health document.
type Health struct {
	Status             string `json:"status"`
	Model              string `json:"model"`
	Variant            string `json:"variant"`
	Backend            string `json:"backend"`
	MaxQuestions       int    `json:"max_questions"`
	PendingRequests    int    `json:"pending_requests"`
	QueuedRequests     int    `json:"queued_requests"`
	Batching           bool   `json:"batching"`
	MaxBatchQuestions  int    `json:"max_batch_questions"`
	MaxPendingRequests int    `json:"max_pending_requests"`
	BatchWaitMS        int    `json:"batch_wait_ms"`
}

// Ready reports whether the engine accepts requests.
func (h Health) Ready() bool { return h.Status == "ok" }

// Health fetches /health.
func (l *Local) Health(ctx context.Context) (Health, error) {
	var h Health
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.baseURL+"/health", nil)
	if err != nil {
		return h, err
	}
	resp, err := l.client.Do(req)
	if err != nil {
		return h, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return h, fmt.Errorf("jev: health: %s", resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
		return h, fmt.Errorf("jev: health: %w", err)
	}
	return h, nil
}

// chunk is one engine-sized slice of a caller request.
type chunk struct {
	owner int // index into the caller's requests
	req   Request
}

// Predict posts to /predict. Requests with more questions than the engine
// accepts per call are split into chunks and their answers merged back.
func (l *Local) Predict(ctx context.Context, reqs []Request) ([]Response, error) {
	chunks := l.split(reqs)
	out := make([]Response, len(reqs))
	for i := range out {
		out[i] = Response{Answers: map[string]Answer{}, Provider: l.Name()}
	}
	for start := 0; start < len(chunks); {
		// Fill one HTTP call up to maxQuestions and maxChars of state, so a
		// batch of long documents never exceeds what the GPU can hold.
		end, total, chars := start, 0, 0
		for end < len(chunks) && total+chunks[end].req.QuestionCount() <= l.maxQuestions {
			c := stateChars(chunks[end].req)
			if end > start && chars+c > l.maxChars {
				break
			}
			total += chunks[end].req.QuestionCount()
			chars += c
			end++
		}
		if end == start {
			end = start + 1
		}
		batch := make([]Request, 0, end-start)
		for _, c := range chunks[start:end] {
			batch = append(batch, c.req)
		}
		data, err := post(ctx, l.client, l.baseURL+"/predict", nil, batch, map[int]bool{http.StatusServiceUnavailable: true}, l.backoff)
		if err != nil {
			return nil, err
		}
		var env struct {
			Results []Response `json:"results"`
		}
		if err := json.Unmarshal(data, &env); err != nil {
			return nil, fmt.Errorf("jev: decode /predict: %w", err)
		}
		if len(env.Results) != len(batch) {
			return nil, fmt.Errorf("jev: /predict returned %d results for %d requests", len(env.Results), len(batch))
		}
		for j, r := range env.Results {
			owner := chunks[start+j].owner
			out[owner].Model = r.Model
			out[owner].Usage.InputTokens += r.Usage.InputTokens
			out[owner].Usage.OutputTokens += r.Usage.OutputTokens
			for id, a := range r.Answers {
				out[owner].Answers[id] = a
			}
		}
		start = end
	}
	return out, nil
}

func (l *Local) split(reqs []Request) []chunk {
	var chunks []chunk
	for i, r := range reqs {
		if r.QuestionCount() <= l.maxQuestions {
			chunks = append(chunks, chunk{owner: i, req: r})
			continue
		}
		for s := 0; s < len(r.Questions); s += l.maxQuestions {
			e := min(s+l.maxQuestions, len(r.Questions))
			part := r
			part.Questions = r.Questions[s:e]
			chunks = append(chunks, chunk{owner: i, req: part})
		}
	}
	return chunks
}
