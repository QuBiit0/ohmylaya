package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuBiit0/ohmylaya/internal/jev"
)

// scriptedTwoOption answers any two-option choice by locating the yes
// description and using the probability from a per-state map.
type scriptedTwoOption struct {
	byState map[string]float64 // substring of state -> p(yes)
	byQ     map[string]float64 // question id -> p(yes), overrides
	calls   int
	batches []int
}

func (s *scriptedTwoOption) Name() string { return "fake" }

func (s *scriptedTwoOption) Predict(ctx context.Context, reqs []jev.Request) ([]jev.Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.calls++
	s.batches = append(s.batches, len(reqs))
	var out []jev.Response
	for _, r := range reqs {
		state, _ := r.State.(string)
		resp := jev.Response{Model: "laya-fake", Answers: map[string]jev.Answer{}, Provider: "fake"}
		for _, q := range r.Questions {
			opts, ok := q.Question.Criteria.(jev.Options)
			if !ok {
				continue
			}
			if len(opts) != 2 {
				// Multi-option classify: pick the option whose key appears in the state.
				probs := map[string]float64{}
				top, best := "", -1.0
				for _, o := range opts {
					p := 0.05
					if strings.Contains(state, o.Key) {
						p = 0.85
					}
					probs[o.Key] = p
					if p > best {
						top, best = o.Key, p
					}
				}
				norm := 0.0
				for _, p := range probs {
					norm += p
				}
				for k := range probs {
					probs[k] /= norm
				}
				conf := 0.9
				resp.Answers[q.ID] = jev.Answer{Type: "choice", Choice: top, Probabilities: probs, Confidence: &conf}
				continue
			}
			p := 0.5
			for sub, v := range s.byState {
				if strings.Contains(state, sub) {
					p = v
				}
			}
			if v, ok := s.byQ[q.ID]; ok {
				p = v
			}
			yesKey, noKey := opts[0].Key, opts[1].Key
			if !strings.HasPrefix(opts[0].Description.(string), "yes") {
				yesKey, noKey = opts[1].Key, opts[0].Key
			}
			top := yesKey
			if p < 0.5 {
				top = noKey
			}
			resp.Answers[q.ID] = choice(top, map[string]float64{yesKey: p, noKey: 1 - p}, 0.2)
		}
		out = append(out, resp)
	}
	return out, nil
}

func TestClassifyLabelsItemsInOrderAndGates(t *testing.T) {
	t.Parallel()
	s := newService(&scriptedTwoOption{})
	out, err := s.Classify(context.Background(), ClassifyInput{
		Items:  []Item{{ID: "i1", Text: "invoice billing question"}, {ID: "i2", Text: "the app crashes, technical"}, {ID: "i3", Text: "nothing matches"}},
		Labels: []Label{{Key: "billing", Description: "payments"}, {Key: "technical", Description: "bugs"}, {Key: "sales"}},
	})
	if err != nil {
		t.Fatalf("Classify() = %v", err)
	}
	if len(out.Results) != 3 || out.Results[0].ID != "i1" || out.Results[0].Label != "billing" || out.Results[1].Label != "technical" {
		t.Errorf("results = %+v", out.Results)
	}
	if out.Results[0].Action != ActionAuto || out.Results[0].Excerpt == "" {
		t.Errorf("r0 = %+v", out.Results[0])
	}
	if _, err := s.Classify(context.Background(), ClassifyInput{Items: []Item{{ID: "x"}}, Labels: []Label{{Key: "a"}}}); err == nil {
		t.Error("one label must be rejected")
	}
}

func TestClassifyFromGlobReadsFilesItself(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("billing refund"), 0o644)
	os.WriteFile(filepath.Join(dir, "sub", "b.md"), []byte("technical outage"), 0o644)
	os.WriteFile(filepath.Join(dir, "sub", "c.txt"), []byte("ignored"), 0o644)
	s := newService(&scriptedTwoOption{})
	out, err := s.Classify(context.Background(), ClassifyInput{Glob: filepath.Join(dir, "**", "*.md"), Labels: []Label{{Key: "billing"}, {Key: "technical"}}})
	if err != nil {
		t.Fatalf("Classify(glob) = %v", err)
	}
	if len(out.Results) != 2 {
		t.Fatalf("results = %+v", out.Results)
	}
	if !strings.HasSuffix(out.Results[0].ID, "a.md") || out.Results[0].Label != "billing" {
		t.Errorf("r0 = %+v", out.Results[0])
	}
}

func TestScreenRecommendations(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		byQ        map[string]float64
		wantAction string
	}{
		{"injection blocks", map[string]float64{"injection": 0.9, "substance": 0.9, "relevance": 0.9}, ScreenBlock},
		{"no substance skips", map[string]float64{"injection": 0.1, "substance": 0.1, "relevance": 0.9}, ScreenSkip},
		{"irrelevant skips", map[string]float64{"injection": 0.1, "substance": 0.9, "relevance": 0.1}, ScreenSkip},
		{"clean allows", map[string]float64{"injection": 0.05, "substance": 0.9, "relevance": 0.8}, ScreenAllow},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := newService(&scriptedTwoOption{byQ: tc.byQ})
			out, err := s.Screen(context.Background(), ScreenInput{Text: "Pricing page content", Purpose: "extract pricing", IncludeText: true})
			if err != nil {
				t.Fatal(err)
			}
			if out.Recommendation.Action != tc.wantAction || out.Recommendation.Reason == "" {
				t.Errorf("recommendation = %+v, want %s", out.Recommendation, tc.wantAction)
			}
			if tc.wantAction == ScreenAllow && out.Text == "" {
				t.Error("allow with include_text must return the text")
			}
			if tc.wantAction != ScreenAllow && out.Text != "" {
				t.Error("blocked or skipped content must not be returned")
			}
		})
	}
}

func TestScreenFromURLStripsHTML(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><head><style>x{}</style><script>evil()</script></head><body><h1>Pricing</h1><p>Starter &amp; Pro</p></body></html>"))
	}))
	t.Cleanup(srv.Close)
	s := newService(&scriptedTwoOption{byQ: map[string]float64{"injection": 0.05, "substance": 0.9, "relevance": 0.9}})
	s.SetReader(NewReader(srv.Client()))
	out, err := s.Screen(context.Background(), ScreenInput{Ref: Ref{URL: srv.URL}, Purpose: "pricing", IncludeText: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.Text, "<") || strings.Contains(out.Text, "evil") || !strings.Contains(out.Text, "Starter & Pro") {
		t.Errorf("text = %q", out.Text)
	}
}

func TestRerankSortsBatchesAndTopK(t *testing.T) {
	t.Parallel()
	p := &scriptedTwoOption{byState: map[string]float64{"best": 0.95, "good": 0.7, "meh": 0.2}}
	s := newService(p)
	var cands []Item
	for i := 0; i < 20; i++ {
		cands = append(cands, Item{ID: "c" + string(rune('a'+i)), Text: "meh " + strings.Repeat("x", i)})
	}
	cands[7].Text = "the best candidate"
	cands[13].Text = "a good one"
	out, err := s.Rerank(context.Background(), RerankInput{Query: "which?", Candidates: cands, TopK: 3})
	if err != nil {
		t.Fatalf("Rerank() = %v", err)
	}
	if len(out.Results) != 3 || out.Results[0].ID != cands[7].ID || out.Results[1].ID != cands[13].ID {
		t.Errorf("results = %+v", out.Results)
	}
	if out.Scored != 20 || out.Total != 20 || out.Meta.Partial {
		t.Errorf("counts = scored %d total %d partial %v", out.Scored, out.Total, out.Meta.Partial)
	}
	if p.calls != 3 || p.batches[0] != rerankBatch {
		t.Errorf("calls = %d batches = %v, want 3 calls of up to %d", p.calls, p.batches, rerankBatch)
	}
	if out.Results[0].Text != "" || out.Results[0].Excerpt == "" {
		t.Error("default output is excerpts, not full text")
	}
}

func TestRerankReturnsPartialOnDeadline(t *testing.T) {
	t.Parallel()
	p := &scriptedTwoOption{}
	s := newService(p)
	var cands []Item
	for i := 0; i < 16; i++ {
		cands = append(cands, Item{ID: string(rune('a' + i)), Text: "t"})
	}
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after the first batch by wrapping the provider.
	s.provider = providerFunc(func(c context.Context, reqs []jev.Request) ([]jev.Response, error) {
		out, err := p.Predict(c, reqs)
		cancel()
		return out, err
	})
	out, err := s.Rerank(ctx, RerankInput{Query: "q", Candidates: cands})
	if err != nil {
		t.Fatalf("Rerank() = %v, want partial results", err)
	}
	if !out.Meta.Partial || out.Scored != 8 || out.Total != 16 {
		t.Errorf("partial = %v scored = %d total = %d", out.Meta.Partial, out.Scored, out.Total)
	}
}

type providerFunc func(context.Context, []jev.Request) ([]jev.Response, error)

func (f providerFunc) Name() string { return "fake" }
func (f providerFunc) Predict(ctx context.Context, reqs []jev.Request) ([]jev.Response, error) {
	return f(ctx, reqs)
}

func TestReaderLimitsAndExcerpt(t *testing.T) {
	t.Parallel()
	r := NewReader(nil)
	big := filepath.Join(t.TempDir(), "big")
	os.WriteFile(big, make([]byte, maxRefBytes+1), 0o644)
	if _, err := r.ReadFile(big); err == nil {
		t.Error("files over the limit must be rejected")
	}
	if _, err := r.Fetch(context.Background(), "ftp://x"); err == nil {
		t.Error("non-http schemes must be rejected")
	}
	if got := Excerpt(strings.Repeat("word ", 100)); len([]rune(got)) > excerptRunes+1 {
		t.Errorf("excerpt too long: %d", len([]rune(got)))
	}
	if _, err := r.Resolve(context.Background(), "", Ref{}); err == nil {
		t.Error("empty inline and empty ref must be rejected")
	}
}
