package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuBiit0/ohmylaya/internal/tools"
)

// fakeRunner answers from canned outputs keyed by case input.
type fakeRunner map[string]string

func (f fakeRunner) run(_ context.Context, _ string, input []byte) ([]byte, error) {
	out, ok := f[string(input)]
	if !ok {
		return nil, errors.New("engine unavailable")
	}
	return []byte(out), nil
}

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return filepath.ToSlash(p)
}

func TestTokensEstimatesFourCharactersPerToken(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]int{"": 0, "abc": 1, "abcd": 1, "abcde": 2, "ññññ": 1} {
		if got := Tokens(in); got != want {
			t.Errorf("Tokens(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestRunScoresEveryTool(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	page := writeFile(t, dir, "page.txt", strings.Repeat("pricing ", 100))
	log := writeFile(t, dir, "log.txt", strings.Repeat("ok ", 100))
	a := writeFile(t, dir, "a.go", strings.Repeat("a", 400))
	b := writeFile(t, dir, "b.go", strings.Repeat("b", 400))
	cases := []struct {
		suite       string
		out         string
		correct     int
		total       int
		baselineMin int
	}{
		{`{"tool":"screen","cases":[{"id":"s","input":{"ref":{"path":"` + page + `"},"purpose":"p"},"want":{"action":"skip"}}]}`,
			`{"recommendation":{"action":"skip"},"meta":{"elapsed_ms":7}}`, 1, 1, 200},
		{`{"tool":"classify","cases":[{"id":"c","input":{"items":[{"id":"1","text":"x"},{"id":"2","text":"y"},{"id":"3","text":"z"}],"labels":[{"key":"bug"},{"key":"docs"}]},"want":{"labels":{"1":"bug","2":"docs","3":"bug"}}}]}`,
			`{"results":[{"id":"1","label":"bug"},{"id":"2","label":"bug"},{"id":"3","label":"bug"}],"meta":{"elapsed_ms":7}}`, 2, 3, 1},
		{`{"tool":"check","cases":[{"id":"k","input":{"claims":["tests pass","build fails"],"ref":{"path":"` + log + `"}},"want":{"verdicts":["supported","contradicted"]}}]}`,
			`{"results":[{"verdict":"supported"},{"verdict":"contradicted"}],"meta":{"elapsed_ms":7}}`, 2, 2, 75},
		{`{"tool":"rerank","cases":[{"id":"r","input":{"query":"q","paths":["` + a + `","` + b + `"],"top_k":1},"want":{"relevant":["` + b + `"]}}]}`,
			`{"results":[{"id":"` + b + `"}],"meta":{"elapsed_ms":7}}`, 1, 1, 200},
	}
	for _, tc := range cases {
		var s Suite
		if err := json.Unmarshal([]byte(tc.suite), &s); err != nil {
			t.Fatal(err)
		}
		t.Run(s.Tool, func(t *testing.T) {
			t.Parallel()
			r := Run(context.Background(), s, fakeRunner{string(s.Cases[0].Input): tc.out}.run, tools.NewReader(nil))
			if r.Correct != tc.correct || r.Total != tc.total {
				t.Errorf("score = %d/%d, want %d/%d (failures %v)", r.Correct, r.Total, tc.correct, tc.total, r.Failures)
			}
			if r.BaselineTokens < tc.baselineMin {
				t.Errorf("BaselineTokens = %d, want at least %d", r.BaselineTokens, tc.baselineMin)
			}
			if want := Tokens(string(s.Cases[0].Input)) + Tokens(tc.out); r.ToolTokens != want {
				t.Errorf("ToolTokens = %d, want %d", r.ToolTokens, want)
			}
			if len(r.ElapsedMS) != 1 || r.ElapsedMS[0] != 7 {
				t.Errorf("ElapsedMS = %v, want [7]", r.ElapsedMS)
			}
		})
	}
}

func TestRunRecordsFailures(t *testing.T) {
	t.Parallel()
	for tool, wantErr := range map[string]string{"check": "engine unavailable", "decide": "not benchmarked"} {
		s := Suite{Tool: tool, Cases: []Case{{ID: "k", Input: json.RawMessage(`{"text":"e"}`), Want: json.RawMessage(`{"verdicts":["supported"]}`)}}}
		r := Run(context.Background(), s, fakeRunner{}.run, tools.NewReader(nil))
		if r.Correct != 0 || len(r.Failures) != 1 || !strings.Contains(r.Failures[0], wantErr) {
			t.Errorf("%s: result = %+v, want a failure containing %q", tool, r, wantErr)
		}
	}
}

func TestReportPrintsOneRowPerTool(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	Report(&buf, []Result{{Tool: "screen", Cases: 2, Correct: 3, Total: 4, BaselineTokens: 1000, ToolTokens: 250, ElapsedMS: []int64{30, 10, 20}}})
	for _, want := range []string{"| screen | 2 | 75% | 1000 | 250 | 75% | 20 |", "| Tool |"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("report lacks %q:\n%s", want, buf.String())
		}
	}
}

func TestLoadSuitesReadsTheCorpus(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, "b.json", `{"tool":"screen","cases":[]}`)
	writeFile(t, dir, "a.json", `{"tool":"check","cases":[]}`)
	writeFile(t, dir, "notes.txt", "ignored")
	suites, err := LoadSuites(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(suites) != 2 || suites[0].Tool != "check" || suites[1].Tool != "screen" {
		t.Fatalf("suites = %+v, want check then screen", suites)
	}
}
