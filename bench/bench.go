package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/QuBiit0/ohmylaya/internal/tools"
)

// Suite is one corpus file: hand-labelled cases for a single tool.
type Suite struct {
	Tool  string `json:"tool"`
	Cases []Case `json:"cases"`
}

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

// Run replays a suite. A case whose call or scoring fails counts as wrong.
func Run(ctx context.Context, s Suite, run Runner, reader *tools.Reader) Result {
	r := Result{Tool: s.Tool, Cases: len(s.Cases)}
	for _, c := range s.Cases {
		if err := r.add(ctx, s.Tool, c, run, reader); err != nil {
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
	content, err := baseline(c.Input, reader)
	if err != nil {
		return err
	}
	r.Total += total
	r.BaselineTokens += Tokens(content)
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
	r.ToolTokens += Tokens(string(c.Input)) + Tokens(compact.String())
	r.Correct += out.correct(tool, w)
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
}

// baseline returns the content an agent would read to answer the case
// itself.
func baseline(raw json.RawMessage, reader *tools.Reader) (string, error) {
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", fmt.Errorf("input: %w", err)
	}
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

// correct counts the labelled decisions the output got right.
func (o output) correct(tool string, w want) int {
	n := 0
	if tool == "screen" && o.Recommendation.Action == w.Action {
		n++
	}
	for i, res := range o.Results {
		switch {
		case tool == "classify" && res.Label != "" && w.Labels[res.ID] == res.Label,
			tool == "check" && i < len(w.Verdicts) && res.Verdict == w.Verdicts[i],
			tool == "rerank" && slices.Contains(w.Relevant, res.ID):
			n++
		}
	}
	return n
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
	return fmt.Sprintf("%d%%", (100*n+d/2)/d)
}

func median(v []int64) int64 {
	if len(v) == 0 {
		return 0
	}
	s := slices.Sorted(slices.Values(v))
	return s[len(s)/2]
}
