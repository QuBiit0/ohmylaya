//go:build engine

package jev

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestLocalAgainstRealEngine runs only with -tags engine and
// OHMYLAYA_TEST_ENGINE_URL pointing at a live laya.cpp server.
func TestLocalAgainstRealEngine(t *testing.T) {
	url := os.Getenv("OHMYLAYA_TEST_ENGINE_URL")
	if url == "" {
		t.Skip("OHMYLAYA_TEST_ENGINE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	p := NewLocal(url, nil, 8)

	h, err := p.Health(ctx)
	if err != nil || !h.Ready() {
		t.Fatalf("health = %+v, %v", h, err)
	}

	var qs Questions
	for i := 0; i < 11; i++ {
		qs = append(qs, QuestionEntry{ID: string(rune('a' + i)), Question: Question{Type: "noul", Instructions: "Does the customer ask for a refund?"}})
	}
	qs = append(qs, QuestionEntry{ID: "dept", Question: Question{Type: "choice", Instructions: "Which department?", Criteria: Options{{"billing", "payments and refunds"}, {"technical", "bugs"}}}})
	out, err := p.Predict(ctx, []Request{{State: "I was charged twice, please refund me.", Questions: qs}})
	if err != nil {
		t.Fatalf("Predict() = %v", err)
	}
	if len(out) != 1 || len(out[0].Answers) != 12 {
		t.Fatalf("expected 12 merged answers, got %d", len(out[0].Answers))
	}
	if out[0].Answers["a"].Noul < 0.9 || out[0].Answers["dept"].Choice != "billing" {
		t.Errorf("unexpected answers: a=%v dept=%q", out[0].Answers["a"].Noul, out[0].Answers["dept"].Choice)
	}
	t.Logf("engine %s backend %s merged 12 questions across split calls", h.Model, h.Backend)
}
