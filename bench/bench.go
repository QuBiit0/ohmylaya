package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuBiit0/ohmylaya/internal/tools"
)

// Suite is one corpus file: hand-labelled cases for a single tool.
type Suite struct {
	Tool    string        `json:"tool"`
	Cases   []Case        `json:"cases"`
	Timeout time.Duration `json:"-"` // per case; zero means defaultTimeout
}

// defaultTimeout covers an engine start plus rerank's 60 second deadline.
const defaultTimeout = 2 * time.Minute

// Case is one tool call and its hand label. Input is passed verbatim to
// `ohmylaya ask`; the shape of Want depends on the tool.
type Case struct {
	ID    string          `json:"id"`
	Input json.RawMessage `json:"input"`
	Want  json.RawMessage `json:"want"`
}

// Runner calls one tool with a JSON input and returns its JSON output.
type Runner func(ctx context.Context, tool string, input []byte) ([]byte, error)

// Result aggregates one suite.
type Result struct {
	Tool           string
	Cases          int
	Correct, Total int // labelled decisions answered correctly, and in all
	BaselineTokens int // content the agent would read without the tool
	ToolTokens     int // the tool call plus its answer
	ElapsedMS      []int64
	Failures       []string
}

// Failed reports whether any case failed; such a run is not publishable.
func (r Result) Failed() bool { return len(r.Failures) > 0 }

// Tokens estimates model tokens as one per four characters. Both sides of
// every comparison use the same estimate, so the ratio is what matters.
func Tokens(s string) int {
	return (utf8.RuneCountInString(s) + 3) / 4
}

// LoadSuites reads every *.json file in dir, in name order (Glob sorts).
func LoadSuites(dir string) ([]Suite, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	suites := make([]Suite, 0, len(paths))
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		var s Suite
		if err := json.Unmarshal(b, &s); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		suites = append(suites, s)
	}
	return suites, nil
}

// Run replays a suite. A failed case counts its decisions as wrong and is
// left out of both token columns, so failures never inflate the savings.
func Run(ctx context.Context, s Suite, run Runner, reader *tools.Reader) Result {
	r := Result{Tool: s.Tool, Cases: len(s.Cases)}
	timeout := cmp.Or(s.Timeout, defaultTimeout)
	for _, c := range s.Cases {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		err := r.add(ctx, s.Tool, c, run, reader)
		cancel()
		if err != nil {
			r.Failures = append(r.Failures, fmt.Sprintf("%s: %v", c.ID, err))
		}
	}
	return r
}

func (r *Result) add(ctx context.Context, tool string, c Case, run Runner, reader *tools.Reader) error {
	var w want
	if err := json.Unmarshal(c.Want, &w); err != nil {
		return fmt.Errorf("want: %w", err)
	}
	total, ok := map[string]int{"screen": 1, "classify": len(w.Labels), "check": len(w.Verdicts), "rerank": len(w.Relevant)}[tool]
	if !ok {
		return fmt.Errorf("tool %s is not benchmarked", tool)
	}
	r.Total += total
	var in input
	if err := json.Unmarshal(c.Input, &in); err != nil {
		return fmt.Errorf("input: %w", err)
	}
	content, err := baseline(in, reader)
	if err != nil {
		return err
	}
	raw, err := run(ctx, tool, c.Input)
	if err != nil {
		return err
	}
	var compact bytes.Buffer
	var out output
	if err := json.Compact(&compact, raw); err != nil {
		return fmt.Errorf("output: %w", err)
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("output: %w", err)
	}
	correct, err := out.correct(tool, in.TopK, w)
	if err != nil {
		return err
	}
	r.Correct += correct
	r.BaselineTokens += Tokens(content)
	r.ToolTokens += Tokens(string(c.Input)) + Tokens(compact.String())
	r.ElapsedMS = append(r.ElapsedMS, out.Meta.ElapsedMS)
	return nil
}

// want holds every tool's label shape; each tool reads its own field.
type want struct {
	Action   string            `json:"action"`
	Labels   map[string]string `json:"labels"`
	Verdicts []string          `json:"verdicts"`
	Relevant []string          `json:"relevant"`
}

// input holds the content fields the corpus uses: inline text or items, a
// file ref, or file paths. URLs are left out so runs are reproducible.
type input struct {
	Text  string       `json:"text"`
	Ref   tools.Ref    `json:"ref"`
	Items []tools.Item `json:"items"`
	Paths []string     `json:"paths"`
	TopK  int          `json:"top_k"`
}

// baseline returns the content an agent would read to answer the case
// itself.
func baseline(in input, reader *tools.Reader) (string, error) {
	parts := []string{in.Text}
	if in.Ref.Path != "" {
		text, err := reader.ReadFile(in.Ref.Path)
		if err != nil {
			return "", err
		}
		parts = append(parts, text)
	}
	files, err := reader.ItemsFromPaths(in.Paths)
	if err != nil {
		return "", err
	}
	for _, it := range append(in.Items, files...) {
		parts = append(parts, it.Text)
	}
	return strings.Join(parts, "\n"), nil
}

// output holds the fields of every tool output that the labels judge.
type output struct {
	Recommendation tools.ScreenRecommendation `json:"recommendation"`
	Results        []struct {
		ID      string `json:"id"`
		Label   string `json:"label"`
		Verdict string `json:"verdict"`
	} `json:"results"`
	Meta tools.Meta `json:"meta"`
}

// correct counts the labelled decisions the output got right. It never
// credits a result past top_k or the same id twice, and it fails when the
// output does not line up with the labels.
func (o output) correct(tool string, topK int, w want) (int, error) {
	if tool == "screen" {
		if o.Recommendation.Action == w.Action {
			return 1, nil
		}
		return 0, nil
	}
	if tool == "check" && len(o.Results) != len(w.Verdicts) {
		return 0, fmt.Errorf("%d verdicts for %d claims", len(o.Results), len(w.Verdicts))
	}
	if tool == "rerank" && topK > 0 && len(o.Results) > topK {
		o.Results = o.Results[:topK]
	}
	n, seen := 0, map[string]bool{}
	for i, res := range o.Results {
		if tool == "check" {
			if res.Verdict == w.Verdicts[i] {
				n++
			}
			continue
		}
		if seen[res.ID] {
			continue
		}
		seen[res.ID] = true
		label, labelled := w.Labels[res.ID]
		switch {
		case tool == "classify" && !labelled:
			return 0, fmt.Errorf("result %s is not labelled", res.ID)
		case tool == "classify" && label == res.Label,
			tool == "rerank" && slices.Contains(w.Relevant, res.ID):
			n++
		}
	}
	return n, nil
}

// Report writes a Markdown table with one row per suite.
func Report(w io.Writer, results []Result) {
	fmt.Fprintln(w, "| Tool | Cases | Accuracy | Tokens without | Tokens with | Saved | Median ms |")
	fmt.Fprintln(w, "|---|---:|---:|---:|---:|---:|---:|")
	for _, r := range results {
		fmt.Fprintf(w, "| %s | %d | %s | %d | %d | %s | %d |\n", r.Tool, r.Cases,
			percent(r.Correct, r.Total), r.BaselineTokens, r.ToolTokens,
			percent(r.BaselineTokens-r.ToolTokens, r.BaselineTokens), median(r.ElapsedMS))
	}
	for _, r := range results {
		for _, f := range r.Failures {
			fmt.Fprintf(w, "\nfailed %s %s", r.Tool, f)
		}
	}
}

func percent(n, d int) string {
	if d == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.0f%%", math.Round(100*float64(n)/float64(d)))
}

func median(v []int64) int64 {
	if len(v) == 0 {
		return 0
	}
	s := slices.Sorted(slices.Values(v))
	return s[len(s)/2]
}
