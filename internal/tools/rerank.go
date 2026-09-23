package tools

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/QuBiit0/ohmylaya/internal/jev"
)

// RerankInput orders candidates by relevance to a query.
type RerankInput struct {
	Query       string   `json:"query" jsonschema:"What you are looking for"`
	Candidates  []Item   `json:"candidates,omitempty" jsonschema:"Candidates as {id, text}, up to 100. Alternatively pass paths or glob"`
	Paths       []string `json:"paths,omitempty"`
	Glob        string   `json:"glob,omitempty" jsonschema:"Glob of files to rank, ** supported; the path is the id"`
	TopK        int      `json:"top_k,omitempty" jsonschema:"Return only the best k (default all)"`
	IncludeText bool     `json:"include_text,omitempty" jsonschema:"Return full text for the returned candidates (default: 200-character excerpts)"`
}

// RerankResult is one scored candidate.
type RerankResult struct {
	ID      string  `json:"id"`
	Score   float64 `json:"score"`
	Excerpt string  `json:"excerpt,omitempty"`
	Text    string  `json:"text,omitempty"`
	Status  string  `json:"status"`
}

// RerankOutput is the result envelope, sorted by score descending.
type RerankOutput struct {
	Results []RerankResult `json:"results"`
	Scored  int            `json:"scored"`
	Total   int            `json:"total"`
	Meta    Meta           `json:"meta"`
}

const (
	maxRerankItems = 100
	rerankDeadline = 60 * time.Second
	rerankBatch    = 8
)

// Rerank scores every candidate with the probability that it is relevant.
// It works in batches and returns partial results when the deadline passes.
func (s *Service) Rerank(ctx context.Context, in RerankInput) (*RerankOutput, error) {
	if in.Query == "" {
		return nil, fmt.Errorf("%w: query is required", ErrInput)
	}
	items, err := s.items(in.Candidates, in.Paths, in.Glob, maxRerankItems)
	if err != nil {
		return nil, err
	}
	started := time.Now()
	ctx, cancel := context.WithTimeout(ctx, rerankDeadline)
	defer cancel()

	yes, opts := s.twoOptions("yes, this candidate answers or is directly useful for the query", "no, this candidate is unrelated or only superficially related to the query")
	q := jev.Question{Type: "choice", Instructions: map[string]any{"question": "Is this candidate relevant to the query?", "query": in.Query}, Criteria: opts}

	out := &RerankOutput{Total: len(items)}
	var pre Preflight
	for start := 0; start < len(items); start += rerankBatch {
		if ctx.Err() != nil {
			out.Meta.Partial = true
			break
		}
		end := min(start+rerankBatch, len(items))
		reqs := make([]jev.Request, 0, end-start)
		for _, it := range items[start:end] {
			pre = mergePreflight(pre, PreflightState(it.Text, s.budget, 2))
			reqs = append(reqs, jev.Request{State: it.Text, Questions: jev.Questions{jev.Q("rel", q)}})
		}
		resp, err := s.provider.Predict(ctx, reqs)
		if err != nil {
			if ctx.Err() != nil && len(out.Results) > 0 {
				out.Meta.Partial = true
				break
			}
			return nil, err
		}
		for i, it := range items[start:end] {
			r := RerankResult{ID: it.ID, Status: StatusInvalid}
			if p, ok := yesProb(resp[i].Answers["rel"], yes); ok {
				r.Score, r.Status = p, StatusOK
			}
			if in.IncludeText {
				r.Text = it.Text
			} else {
				r.Excerpt = Excerpt(it.Text)
			}
			out.Results = append(out.Results, r)
		}
		if out.Meta.Model == "" && len(resp) > 0 {
			out.Meta.Model = resp[0].Model
		}
	}
	out.Scored = len(out.Results)
	sort.SliceStable(out.Results, func(i, j int) bool { return out.Results[i].Score > out.Results[j].Score })
	if in.TopK > 0 && in.TopK < len(out.Results) {
		out.Results = out.Results[:in.TopK]
	}
	partial := out.Meta.Partial
	model := out.Meta.Model
	out.Meta = metaFrom(pre, nil, s.provider.Name(), started)
	out.Meta.Partial = partial
	out.Meta.Model = model
	return out, nil
}
