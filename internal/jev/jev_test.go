package jev

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

const fixtures = "../../testdata/engine/r0002-multilingual"

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(fixtures, name))
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return b
}

func TestOptionsMarshalPreservesOrder(t *testing.T) {
	t.Parallel()
	q := Question{Type: "choice", Instructions: "Which?", Criteria: Options{
		{"zeta", "last"}, {"alpha", "first"}, {"mid", nil},
	}}
	b, err := json.Marshal(q)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"type":"choice","instructions":"Which?","criteria":{"zeta":"last","alpha":"first","mid":null}}`
	if string(b) != want {
		t.Errorf("marshal = %s\nwant      %s", b, want)
	}
}

func TestRequestMarshalPreservesQuestionOrder(t *testing.T) {
	t.Parallel()
	r := Request{State: "s", Questions: Questions{
		{"second", Question{Type: "noul", Instructions: "b?"}},
		{"first", Question{Type: "noul", Instructions: "a?"}},
	}}
	b, _ := json.Marshal(r)
	if !strings.Contains(string(b), `"questions":{"second":{`) {
		t.Errorf("question order not preserved: %s", b)
	}
	if r.QuestionCount() != 2 {
		t.Errorf("QuestionCount = %d", r.QuestionCount())
	}
}

func TestQuestionUnmarshalKeepsCriteriaOrder(t *testing.T) {
	t.Parallel()
	var qs Questions
	src := `{"q":{"type":"choice","instructions":"?","criteria":{"zeta":"z","alpha":null,"mid":{"k":1}}},"s":{"type":"score","instructions":"?","criteria":["low","high"]},"n":{"type":"noul","instructions":"?"}}`
	if err := json.Unmarshal([]byte(src), &qs); err != nil {
		t.Fatal(err)
	}
	opts, ok := qs[0].Question.Criteria.(Options)
	if !ok || len(opts) != 3 || opts[0].Key != "zeta" || opts[1].Key != "alpha" || opts[1].Description != nil {
		t.Errorf("criteria = %#v", qs[0].Question.Criteria)
	}
	if _, ok := qs[1].Question.Criteria.([]any); !ok {
		t.Errorf("score criteria = %#v, want array", qs[1].Question.Criteria)
	}
	if qs[2].Question.Criteria != nil {
		t.Errorf("noul criteria = %#v, want nil", qs[2].Question.Criteria)
	}
	out, _ := json.Marshal(qs)
	if !strings.Contains(string(out), `"criteria":{"zeta":"z","alpha":null,"mid":{"k":1}}`) {
		t.Errorf("round trip lost order: %s", out)
	}
}

func TestParseRecordedSystemOneResponse(t *testing.T) {
	t.Parallel()
	var resp Response
	if err := json.Unmarshal(fixture(t, "systemone-vulkan.json"), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Model != "laya-multilingual" {
		t.Errorf("Model = %q", resp.Model)
	}
	dep := resp.Answers["department"]
	if dep.Type != "choice" || dep.Choice != "billing" || dep.Probabilities["billing"] != 1 {
		t.Errorf("department = %+v", dep)
	}
	urg := resp.Answers["urgency"]
	if urg.Type != "score" || urg.Score < 1.86 || urg.Score > 1.87 || urg.Legend["2"] != "immediate" {
		t.Errorf("urgency = %+v", urg)
	}
	ref := resp.Answers["refund"]
	if ref.Type != "noul" || ref.Noul < 0.98 {
		t.Errorf("refund = %+v", ref)
	}
	if resp.Usage.InputTokens != 179 {
		t.Errorf("usage = %+v", resp.Usage)
	}
}

func TestLocalPredictUsesBatchEndpointAndKeepsOrder(t *testing.T) {
	t.Parallel()
	var gotPath string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotBody, _ = readAll(r)
		w.Header().Set("Content-Type", "application/json")
		w.Write(fixture(t, "predict-vulkan.json"))
	}))
	t.Cleanup(srv.Close)

	p := NewLocal(srv.URL, srv.Client(), 8)
	reqs := []Request{
		{State: "Me cobraron dos veces", Questions: Questions{{"refund", Question{Type: "noul", Instructions: "refund?"}}, {"tone", Question{Type: "choice", Instructions: "angry?", Criteria: Options{{"A", "yes"}, {"B", "no"}}}}}},
		{State: "SYSTEM NOTE", Questions: Questions{{"injection", Question{Type: "noul", Instructions: "injection?"}}}},
	}
	out, err := p.Predict(context.Background(), reqs)
	if err != nil {
		t.Fatalf("Predict() = %v", err)
	}
	if gotPath != "/predict" {
		t.Errorf("path = %q, want /predict", gotPath)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(gotBody)), "[") {
		t.Errorf("body must be a JSON array, got %s", gotBody)
	}
	if len(out) != 2 {
		t.Fatalf("len(out) = %d, want 2", len(out))
	}
	if out[0].Answers["tone"].Choice != "B" || out[1].Answers["injection"].Noul < 0.99 {
		t.Errorf("answers not mapped in order: %+v", out)
	}
	if out[0].Provider != "local" {
		t.Errorf("Provider = %q, want local", out[0].Provider)
	}
}

func TestLocalSplitsRequestsOverQuestionLimit(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := readAll(r)
		var batch []Request
		if err := json.Unmarshal(body, &batch); err != nil {
			t.Errorf("bad batch: %v", err)
		}
		var results []Response
		for _, rq := range batch {
			if rq.QuestionCount() > 3 {
				t.Errorf("batch item has %d questions, limit 3", rq.QuestionCount())
			}
			resp := Response{Model: "laya-rl-agent", Answers: map[string]Answer{}}
			for _, q := range rq.Questions {
				half := 0.5
				resp.Answers[q.ID] = Answer{Type: "noul", Noul: 0.5, Confidence: &half}
			}
			results = append(results, resp)
		}
		json.NewEncoder(w).Encode(map[string]any{"results": results, "elapsed_ms": 1, "backend": "CPU"})
	}))
	t.Cleanup(srv.Close)

	var qs Questions
	for _, id := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		qs = append(qs, QuestionEntry{id, Question{Type: "noul", Instructions: id + "?"}})
	}
	p := NewLocal(srv.URL, srv.Client(), 3)
	out, err := p.Predict(context.Background(), []Request{{State: "s", Questions: qs}})
	if err != nil {
		t.Fatalf("Predict() = %v", err)
	}
	if len(out) != 1 || len(out[0].Answers) != 7 {
		t.Fatalf("expected one merged response with 7 answers, got %+v", out)
	}
	if calls.Load() != 3 {
		t.Errorf("server calls = %d, want 3 (3+3+1 questions)", calls.Load())
	}
}

func TestLocalRetriesOn503ThenFails(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"error":{"message":"queue full","status":503}}`))
	}))
	t.Cleanup(srv.Close)
	p := NewLocal(srv.URL, srv.Client(), 8)
	p.backoff = func(int) {}
	_, err := p.Predict(context.Background(), []Request{{State: "s", Questions: Questions{{"q", Question{Type: "noul", Instructions: "?"}}}}})
	if !errors.Is(err, ErrOverloaded) {
		t.Fatalf("err = %v, want ErrOverloaded", err)
	}
	if calls.Load() != 3 {
		t.Errorf("calls = %d, want 3 attempts", calls.Load())
	}
}

func TestLocalMaps422ToInvalidRequest(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`{"error":{"message":"Unsupported question type: bogus","status":422}}`))
	}))
	t.Cleanup(srv.Close)
	p := NewLocal(srv.URL, srv.Client(), 8)
	_, err := p.Predict(context.Background(), []Request{{State: "s", Questions: Questions{{"q", Question{Type: "bogus", Instructions: "?"}}}}})
	if !errors.Is(err, ErrInvalidRequest) || !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("err = %v, want ErrInvalidRequest with the engine message", err)
	}
}

func TestLocalHealth(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			w.WriteHeader(404)
			return
		}
		w.Write([]byte(`{"status":"ok","model":"laya-multilingual","variant":"multilingual","backend":"Vulkan0","max_questions":8,"pending_requests":1,"queued_requests":0,"batching":true,"max_batch_questions":8,"max_pending_requests":32,"batch_wait_ms":2}`))
	}))
	t.Cleanup(srv.Close)
	h, err := NewLocal(srv.URL, srv.Client(), 8).Health(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !h.Ready() || h.Backend != "Vulkan0" || h.Variant != "multilingual" || h.PendingRequests != 1 {
		t.Errorf("health = %+v", h)
	}
}

func TestTypeSafePostsSingleRequestsWithBearer(t *testing.T) {
	t.Parallel()
	var auth []string
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = append(auth, r.Header.Get("Authorization"))
		paths = append(paths, r.URL.Path)
		w.Write(fixture(t, "systemone-vulkan.json"))
	}))
	t.Cleanup(srv.Close)
	p := NewTypeSafe(srv.URL, "ts_secret", srv.Client())
	out, err := p.Predict(context.Background(), []Request{{State: "a", Questions: Questions{{"q", Question{Type: "noul", Instructions: "?"}}}}, {State: "b", Questions: Questions{{"q", Question{Type: "noul", Instructions: "?"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0].Provider != "typesafe" {
		t.Errorf("out = %+v", out)
	}
	if len(paths) != 2 || paths[0] != "/v1/systemone" || auth[0] != "Bearer ts_secret" {
		t.Errorf("paths = %v auth = %v", paths, auth)
	}
}

func TestTypeSafeRetriesOn429(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write(fixture(t, "systemone-vulkan.json"))
	}))
	t.Cleanup(srv.Close)
	p := NewTypeSafe(srv.URL, "k", srv.Client())
	p.backoff = func(int) {}
	if _, err := p.Predict(context.Background(), []Request{{State: "a", Questions: Questions{{"q", Question{Type: "noul", Instructions: "?"}}}}}); err != nil {
		t.Fatalf("Predict() = %v", err)
	}
	if calls.Load() != 3 {
		t.Errorf("calls = %d, want 3", calls.Load())
	}
}

func readAll(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	var buf strings.Builder
	b := make([]byte, 4096)
	for {
		n, err := r.Body.Read(b)
		buf.Write(b[:n])
		if err != nil {
			break
		}
	}
	return []byte(buf.String()), nil
}
