package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/QuBiit0/ohmylaya/internal/jev"
	"github.com/QuBiit0/ohmylaya/internal/tools"
)

// fakeProvider answers two-option choices with a fixed yes probability and
// multi-option choices with the first option.
type fakeProvider struct{ yes float64 }

func (f fakeProvider) Name() string { return "fake" }
func (f fakeProvider) Predict(_ context.Context, reqs []jev.Request) ([]jev.Response, error) {
	var out []jev.Response
	for _, r := range reqs {
		resp := jev.Response{Model: "laya-fake", Answers: map[string]jev.Answer{}, Provider: "fake"}
		for _, q := range r.Questions {
			conf := 0.9
			switch q.Question.Type {
			case "noul":
				resp.Answers[q.ID] = jev.Answer{Type: "noul", Noul: f.yes, Confidence: &conf}
			case "choice":
				opts := q.Question.Criteria.(jev.Options)
				if len(opts) == 2 {
					yesKey, noKey := opts[0].Key, opts[1].Key
					if d, ok := opts[0].Description.(string); ok && !strings.HasPrefix(d, "yes") {
						yesKey, noKey = noKey, yesKey
					}
					top := yesKey
					if f.yes < 0.5 {
						top = noKey
					}
					resp.Answers[q.ID] = jev.Answer{Type: "choice", Choice: top, Probabilities: map[string]float64{yesKey: f.yes, noKey: 1 - f.yes}, Confidence: &conf}
					continue
				}
				probs := map[string]float64{}
				for i, o := range opts {
					if i == 0 {
						probs[o.Key] = 0.9
					} else {
						probs[o.Key] = 0.1 / float64(len(opts)-1)
					}
				}
				resp.Answers[q.ID] = jev.Answer{Type: "choice", Choice: opts[0].Key, Probabilities: probs, Confidence: &conf}
			}
		}
		out = append(out, resp)
	}
	return out, nil
}

type fakeEngine struct {
	svc *tools.Service
	err error
	hit int
}

func (e *fakeEngine) Service(ctx context.Context) (*tools.Service, error) {
	e.hit++
	return e.svc, e.err
}
func (e *fakeEngine) Touch() {}

func connect(t *testing.T, eng Engine) *mcp.ClientSession {
	t.Helper()
	srv := New(eng, "test")
	st, ct := mcp.NewInMemoryTransports()
	if _, err := srv.Connect(context.Background(), st, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "0"}, nil)
	cs, err := client.Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func TestListsFiveToolsWithLimits(t *testing.T) {
	cs := connect(t, &fakeEngine{svc: tools.NewService(fakeProvider{yes: 0.9}, "multilingual", 0.8)})
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tl := range res.Tools {
		names[tl.Name] = true
		if !strings.Contains(tl.Description, "Limits:") {
			t.Errorf("tool %s description lacks a Limits sentence", tl.Name)
		}
	}
	for _, want := range []string{"decide", "classify", "check", "screen", "rerank"} {
		if !names[want] {
			t.Errorf("missing tool %s", want)
		}
	}
	if len(res.Tools) != 5 {
		t.Errorf("tools = %d, want 5", len(res.Tools))
	}
}

func TestDecideRoundTripKeepsQuestionOrder(t *testing.T) {
	cs := connect(t, &fakeEngine{svc: tools.NewService(fakeProvider{yes: 0.9}, "multilingual", 0.8)})
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "decide", Arguments: map[string]any{
		"state":     "I was charged twice",
		"questions": json.RawMessage(`{"z_first":{"type":"choice","instructions":"dept?","criteria":{"zeta":"z","alpha":"a"}},"a_second":{"type":"noul","instructions":"refund?"}}`),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("tool error: %s", text(res))
	}
	var out tools.DecideOutput
	if err := json.Unmarshal([]byte(text(res)), &out); err != nil {
		t.Fatalf("decode: %v (%s)", err, text(res))
	}
	if out.Answers["z_first"].Choice != "zeta" {
		t.Errorf("option order lost: %+v", out.Answers["z_first"])
	}
	if out.Answers["a_second"].PYes != 0.9 || out.Answers["a_second"].Action != "auto" {
		t.Errorf("noul = %+v", out.Answers["a_second"])
	}
}

func TestCheckAndScreenAndRerankAndClassify(t *testing.T) {
	cs := connect(t, &fakeEngine{svc: tools.NewService(fakeProvider{yes: 0.2}, "multilingual", 0.8)})
	ctx := context.Background()

	res, _ := cs.CallTool(ctx, &mcp.CallToolParams{Name: "check", Arguments: map[string]any{"claims": []string{"tests pass"}, "evidence": "FAIL"}})
	if res.IsError || !strings.Contains(text(res), `"verdict":"contradicted"`) {
		t.Errorf("check = %s", text(res))
	}
	res, _ = cs.CallTool(ctx, &mcp.CallToolParams{Name: "screen", Arguments: map[string]any{"text": "hello", "purpose": "p"}})
	if res.IsError || !strings.Contains(text(res), `"action":"skip"`) {
		t.Errorf("screen = %s", text(res))
	}
	res, _ = cs.CallTool(ctx, &mcp.CallToolParams{Name: "rerank", Arguments: map[string]any{"query": "q", "candidates": []map[string]string{{"id": "a", "text": "x"}, {"id": "b", "text": "y"}}, "top_k": 1}})
	if res.IsError || !strings.Contains(text(res), `"scored":2`) {
		t.Errorf("rerank = %s", text(res))
	}
	res, _ = cs.CallTool(ctx, &mcp.CallToolParams{Name: "classify", Arguments: map[string]any{"items": []map[string]string{{"id": "a", "text": "x"}}, "labels": []map[string]string{{"key": "one"}, {"key": "two"}, {"key": "three"}}}})
	if res.IsError || !strings.Contains(text(res), `"label":"one"`) {
		t.Errorf("classify = %s", text(res))
	}
}

func TestEngineFailureIsToolErrorWithDoctorHint(t *testing.T) {
	eng := &fakeEngine{err: errors.New("engine exited: simulated crash")}
	cs := connect(t, eng)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "check", Arguments: map[string]any{"claims": []string{"c"}, "evidence": "e"}})
	if err != nil {
		t.Fatalf("protocol error = %v, want a tool error", err)
	}
	if !res.IsError || !strings.Contains(text(res), "ohmylaya doctor") || !strings.Contains(text(res), "simulated crash") {
		t.Errorf("result = %s", text(res))
	}
}

func TestInvalidInputIsToolError(t *testing.T) {
	cs := connect(t, &fakeEngine{svc: tools.NewService(fakeProvider{yes: 0.9}, "multilingual", 0.8)})
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "check", Arguments: map[string]any{"claims": []string{}, "evidence": "e"}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(text(res), "claim") {
		t.Errorf("result = %s", text(res))
	}
}

func text(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}
