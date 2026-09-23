package tools

import (
	"context"
	"errors"
	"math/rand"
	"strings"
	"testing"

	"github.com/QuBiit0/ohmylaya/internal/jev"
)

// fakeProvider answers every question from a script keyed by question id,
// or by instructions when no id matches, and records requests.
type fakeProvider struct {
	answers  map[string]jev.Answer
	requests []jev.Request
	err      error
}

func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) Predict(_ context.Context, reqs []jev.Request) ([]jev.Response, error) {
	f.requests = append(f.requests, reqs...)
	if f.err != nil {
		return nil, f.err
	}
	out := make([]jev.Response, 0, len(reqs))
	for _, r := range reqs {
		resp := jev.Response{Model: "laya-fake", Answers: map[string]jev.Answer{}, Provider: "fake"}
		for _, q := range r.Questions {
			if a, ok := f.answers[q.ID]; ok {
				resp.Answers[q.ID] = a
				continue
			}
			if a, ok := f.answers[fmtInstr(q.Question.Instructions)]; ok {
				resp.Answers[q.ID] = a
			}
		}
		out = append(out, resp)
	}
	return out, nil
}

func fmtInstr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func fptr(v float64) *float64 { return &v }

func choice(top string, probs map[string]float64, conf float64) jev.Answer {
	return jev.Answer{Type: "choice", Choice: top, Probabilities: probs, Confidence: fptr(conf)}
}

func newService(p jev.Provider) *Service {
	s := NewService(p, "multilingual", 0.8)
	s.rand = rand.New(rand.NewSource(1))
	return s
}

func TestBudgetAndPreflight(t *testing.T) {
	t.Parallel()
	b := BudgetFor("multilingual")
	if b.StateTokens() != 1024-256 {
		t.Errorf("multilingual state budget = %d", b.StateTokens())
	}
	if BudgetFor("english").StateTokens() != 512-192 {
		t.Error("english state budget")
	}

	short := PreflightState("hello world", b, 2)
	if short.Truncated || short.EstimatedTokens == 0 {
		t.Errorf("short preflight = %+v", short)
	}
	long := PreflightState(strings.Repeat("palabra ", 2000), b, 2)
	if !long.Truncated || long.EstimatedDroppedChars <= 0 {
		t.Errorf("long preflight = %+v", long)
	}
	many := PreflightState("x", b, 30)
	if len(many.Warnings) == 0 || !strings.Contains(many.Warnings[0], "20") {
		t.Errorf("expected option-count warning, got %+v", many.Warnings)
	}
	obj := PreflightState(map[string]any{"subject": "a", "body": strings.Repeat("b", 5000)}, b, 2)
	if !obj.Truncated {
		t.Error("object state must be measured by its JSON size")
	}
}

func TestGate(t *testing.T) {
	t.Parallel()
	g := Gate{AutoAccept: 0.8}
	if g.Action(0.8) != ActionAuto || g.Action(0.79) != ActionReview {
		t.Error("gate threshold is inclusive at auto_accept")
	}
	if g.With(fptr(0.5)).Action(0.6) != ActionAuto {
		t.Error("per-call override must apply")
	}
}

func TestDecideMapsEveryTypeAndGates(t *testing.T) {
	t.Parallel()
	p := &fakeProvider{answers: map[string]jev.Answer{
		"dept":   choice("billing", map[string]float64{"billing": 0.9, "sales": 0.1}, 0.85),
		"urg":    {Type: "score", Score: 1.86, Probabilities: map[string]float64{"0": 0.01, "1": 0.12, "2": 0.87}, Legend: map[string]string{"0": "low", "2": "high"}, Confidence: fptr(0.62)},
		"refund": {Type: "noul", Noul: 0.98, Confidence: fptr(0.98)},
		"broken": {Type: "choice", Choice: "x", Probabilities: map[string]float64{"x": 1}},
	}}
	s := newService(p)
	out, err := s.Decide(context.Background(), DecideInput{
		State: "I was charged twice",
		Questions: jev.Questions{
			jev.Q("dept", jev.Question{Type: "choice", Instructions: "dept?", Criteria: jev.Options{jev.Opt("billing", "b"), jev.Opt("sales", "s")}}),
			jev.Q("urg", jev.Question{Type: "score", Instructions: "urgent?", Criteria: []string{"low", "mid", "high"}}),
			jev.Q("refund", jev.Question{Type: "noul", Instructions: "refund?"}),
			jev.Q("broken", jev.Question{Type: "choice", Instructions: "?", Criteria: jev.Options{jev.Opt("x", nil), jev.Opt("y", nil)}}),
		},
	})
	if err != nil {
		t.Fatalf("Decide() = %v", err)
	}
	if a := out.Answers["dept"]; a.Choice != "billing" || a.Action != ActionAuto || a.Status != StatusOK {
		t.Errorf("dept = %+v", a)
	}
	if a := out.Answers["urg"]; a.Score != 1.86 || a.Action != ActionReview {
		t.Errorf("urg = %+v (score gates on engine confidence)", a)
	}
	if a := out.Answers["refund"]; a.PYes != 0.98 || a.Action != ActionAuto {
		t.Errorf("refund = %+v", a)
	}
	if a := out.Answers["broken"]; a.Status != StatusInvalid || a.Action != ActionReview {
		t.Errorf("broken = %+v, want invalid_response because confidence is missing", a)
	}
	if out.Meta.Provider != "fake" || out.Meta.Model != "laya-fake" {
		t.Errorf("meta = %+v", out.Meta)
	}
}

func TestDecideRejectsEmptyQuestionsAndPropagatesProviderErrors(t *testing.T) {
	t.Parallel()
	s := newService(&fakeProvider{})
	if _, err := s.Decide(context.Background(), DecideInput{State: "x"}); !errors.Is(err, ErrInput) {
		t.Errorf("empty questions err = %v, want ErrInput", err)
	}
	s = newService(&fakeProvider{err: jev.ErrOverloaded})
	_, err := s.Decide(context.Background(), DecideInput{State: "x", Questions: jev.Questions{jev.Q("q", jev.Question{Type: "noul", Instructions: "?"})}})
	if !errors.Is(err, jev.ErrOverloaded) {
		t.Errorf("err = %v, want provider error", err)
	}
}

func TestCheckUsesTwoOptionChoiceWithRandomKeys(t *testing.T) {
	t.Parallel()
	p := &fakeProvider{answers: map[string]jev.Answer{}}
	s := newService(p)
	// Script answers by inspecting the requests: whichever key carries the
	// "yes" description gets 0.9 for claim 0 and 0.2 for claim 1.
	p2 := &scriptedCheck{yesProb: []float64{0.9, 0.2}}
	s.provider = p2
	out, err := s.Check(context.Background(), CheckInput{
		Claims:   []string{"all tests passed", "the build used Go 1.26", "c3", "c4", "c5", "c6", "c7", "c8"},
		Evidence: "go test ./... ok; FAIL internal/x",
	})
	if err != nil {
		t.Fatalf("Check() = %v", err)
	}
	if len(out.Results) != 8 {
		t.Fatalf("results = %d", len(out.Results))
	}
	r0, r1 := out.Results[0], out.Results[1]
	if r0.Verdict != VerdictSupported || r0.PYes != 0.9 || r0.Action != ActionAuto {
		t.Errorf("r0 = %+v", r0)
	}
	if r1.Verdict != VerdictContradicted || r1.PYes != 0.2 || r1.Action != ActionAuto {
		t.Errorf("r1 = %+v (top probability 0.8 gates auto)", r1)
	}
	if out.Summary.Supported != 4 || out.Summary.Contradicted != 4 {
		t.Errorf("summary = %+v", out.Summary)
	}
	for _, q := range p2.seen {
		if q.Type != "choice" {
			t.Errorf("check must ask a choice, got %s", q.Type)
		}
		opts, ok := q.Criteria.(jev.Options)
		if !ok || len(opts) != 2 {
			t.Fatalf("criteria = %#v, want two options", q.Criteria)
		}
		if opts[0].Key == "yes" || opts[0].Key == "true" {
			t.Error("keys must be neutral, not yes/no")
		}
	}
	if !p2.sawYesFirst || !p2.sawNoFirst {
		t.Error("expected the yes option to appear in both positions across claims (randomised keys)")
	}
}

// scriptedCheck answers two-option choices by locating the yes description.
type scriptedCheck struct {
	yesProb     []float64
	seen        []jev.Question
	sawYesFirst bool
	sawNoFirst  bool
	i           int
}

func (s *scriptedCheck) Name() string { return "fake" }

func (s *scriptedCheck) Predict(_ context.Context, reqs []jev.Request) ([]jev.Response, error) {
	var out []jev.Response
	for _, r := range reqs {
		resp := jev.Response{Model: "laya-fake", Answers: map[string]jev.Answer{}, Provider: "fake"}
		for _, q := range r.Questions {
			s.seen = append(s.seen, q.Question)
			opts := q.Question.Criteria.(jev.Options)
			yesKey, noKey := opts[0].Key, opts[1].Key
			if strings.HasPrefix(opts[0].Description.(string), "yes") {
				s.sawYesFirst = true
			} else {
				s.sawNoFirst = true
				yesKey, noKey = opts[1].Key, opts[0].Key
			}
			p := s.yesProb[s.i%len(s.yesProb)]
			s.i++
			top := yesKey
			if p < 0.5 {
				top = noKey
			}
			resp.Answers[q.ID] = choice(top, map[string]float64{yesKey: p, noKey: 1 - p}, 0.1)
		}
		out = append(out, resp)
	}
	return out, nil
}

func TestCheckPreflightReportsTruncation(t *testing.T) {
	t.Parallel()
	s := newService(&scriptedCheck{yesProb: []float64{0.9}})
	out, err := s.Check(context.Background(), CheckInput{Claims: []string{"c"}, Evidence: strings.Repeat("evidence ", 3000)})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Meta.Truncated || out.Meta.EstimatedDroppedChars == 0 {
		t.Errorf("meta = %+v, want truncation flagged", out.Meta)
	}
}
