package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/QuBiit0/ohmylaya/internal/jev"
)

// Label is one classification target.
type Label struct {
	Key         string `json:"key"`
	Description string `json:"description,omitempty"`
}

// ClassifyInput assigns each item to one label.
type ClassifyInput struct {
	Items        []Item   `json:"items,omitempty" jsonschema:"Items to classify as {id, text}. Alternatively pass paths or glob"`
	Paths        []string `json:"paths,omitempty" jsonschema:"Files to classify, read by ohmylaya"`
	Glob         string   `json:"glob,omitempty" jsonschema:"Glob of files to classify, ** supported"`
	Labels       []Label  `json:"labels" jsonschema:"Ordered labels as {key, description}; keep under 20 and describe each one"`
	Instructions string   `json:"instructions,omitempty" jsonschema:"What to decide, default: which label fits this item best"`
	AutoAccept   *float64 `json:"auto_accept,omitempty"`
}

// ClassifyResult is one labelled item.
type ClassifyResult struct {
	ID            string             `json:"id"`
	Label         string             `json:"label"`
	Confidence    float64            `json:"confidence"`
	Probabilities map[string]float64 `json:"probabilities"`
	Action        string             `json:"action"`
	Status        string             `json:"status"`
	Excerpt       string             `json:"excerpt,omitempty"`
}

// ClassifyOutput is the result envelope.
type ClassifyOutput struct {
	Results []ClassifyResult `json:"results"`
	Meta    Meta             `json:"meta"`
}

const maxClassifyItems = 50

// Classify labels up to 50 items against a shared catalogue.
func (s *Service) Classify(ctx context.Context, in ClassifyInput) (*ClassifyOutput, error) {
	if len(in.Labels) < 2 {
		return nil, fmt.Errorf("%w: at least two labels are required", ErrInput)
	}
	items, err := s.items(in.Items, in.Paths, in.Glob, maxClassifyItems)
	if err != nil {
		return nil, err
	}
	instr := in.Instructions
	if instr == "" {
		instr = "Which label fits this item best?"
	}
	opts := make(jev.Options, 0, len(in.Labels))
	for _, l := range in.Labels {
		var desc any
		if l.Description != "" {
			desc = l.Description
		}
		opts = append(opts, jev.Opt(l.Key, desc))
	}
	started := time.Now()
	reqs := make([]jev.Request, 0, len(items))
	var pre Preflight
	for _, it := range items {
		p := PreflightState(it.Text, s.budget, len(in.Labels))
		pre = mergePreflight(pre, p)
		reqs = append(reqs, jev.Request{State: it.Text, Questions: jev.Questions{jev.Q("label", jev.Question{Type: "choice", Instructions: instr, Criteria: opts})}})
	}
	resp, err := s.provider.Predict(ctx, reqs)
	if err != nil {
		return nil, err
	}
	gate := s.gate.With(in.AutoAccept)
	out := &ClassifyOutput{Meta: metaFrom(pre, resp, s.provider.Name(), started)}
	for i, it := range items {
		r := ClassifyResult{ID: it.ID, Action: ActionReview, Status: StatusInvalid, Excerpt: Excerpt(it.Text)}
		if a, ok := resp[i].Answers["label"]; ok {
			g := gateAnswer(a, gate)
			r.Label, r.Probabilities, r.Action, r.Status = g.Choice, g.Probabilities, g.Action, g.Status
			if len(a.Probabilities) == 2 {
				r.Confidence = topProbability(a)
			} else {
				r.Confidence = a.ConfidenceValue()
			}
		}
		out.Results = append(out.Results, r)
	}
	return out, nil
}

// items resolves inline items, explicit paths or a glob, in that order.
func (s *Service) items(inline []Item, paths []string, glob string, limit int) ([]Item, error) {
	var items []Item
	var err error
	switch {
	case len(inline) > 0:
		items = inline
	case len(paths) > 0:
		items, err = s.reader.ItemsFromPaths(paths)
	case glob != "":
		items, err = s.reader.ItemsFromGlob(glob, limit)
	default:
		return nil, fmt.Errorf("%w: provide items, paths or glob", ErrInput)
	}
	if err != nil {
		return nil, err
	}
	if len(items) > limit {
		return nil, fmt.Errorf("%w: %d items exceed the limit of %d", ErrInput, len(items), limit)
	}
	for i, it := range items {
		if it.ID == "" {
			return nil, fmt.Errorf("%w: item %d has no id", ErrInput, i)
		}
	}
	return items, nil
}

func mergePreflight(acc, p Preflight) Preflight {
	if p.Truncated {
		acc.Truncated = true
		acc.EstimatedDroppedChars += p.EstimatedDroppedChars
	}
	if acc.StateBudget == 0 {
		acc.StateBudget = p.StateBudget
	}
	for _, w := range p.Warnings {
		if !containsWarning(acc.Warnings, w) {
			acc.Warnings = append(acc.Warnings, w)
		}
	}
	return acc
}

func containsWarning(list []string, w string) bool {
	for _, v := range list {
		if v == w {
			return true
		}
	}
	return false
}
