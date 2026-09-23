package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/QuBiit0/ohmylaya/internal/jev"
)

// CheckInput verifies claims against evidence.
type CheckInput struct {
	Claims     []string `json:"claims" jsonschema:"Claims to verify, one sentence each"`
	Evidence   any      `json:"evidence,omitempty" jsonschema:"The evidence text or object. Alternatively pass path or url"`
	Ref        Ref      `json:"ref,omitempty" jsonschema:"Optional reference to read the evidence from: {path} or {url}"`
	AutoAccept *float64 `json:"auto_accept,omitempty" jsonschema:"Override the top-probability threshold for action=auto"`
}

// CheckResult is one verified claim.
type CheckResult struct {
	Claim      string  `json:"claim"`
	Verdict    string  `json:"verdict"`
	PYes       float64 `json:"p_yes"`
	Confidence float64 `json:"confidence"`
	Action     string  `json:"action"`
	Status     string  `json:"status"`
}

// CheckSummary counts verdicts.
type CheckSummary struct {
	Supported    int `json:"supported"`
	Contradicted int `json:"contradicted"`
	Review       int `json:"review"`
	Invalid      int `json:"invalid"`
}

// CheckOutput is the result envelope.
type CheckOutput struct {
	Results []CheckResult `json:"results"`
	Summary CheckSummary  `json:"summary"`
	Meta    Meta          `json:"meta"`
}

// Check asks, for every claim, whether the evidence supports it. Upstream
// documents that the noul primitive can follow its own labels, so each claim
// is asked as a two-option choice with neutral keys whose order is random.
func (s *Service) Check(ctx context.Context, in CheckInput) (*CheckOutput, error) {
	if len(in.Claims) == 0 {
		return nil, fmt.Errorf("%w: at least one claim is required", ErrInput)
	}
	evidence, err := s.reader.Resolve(ctx, in.Evidence, in.Ref)
	if err != nil {
		return nil, err
	}
	started := time.Now()
	pre := PreflightState(evidence, s.budget, 2)

	qs := make(jev.Questions, 0, len(in.Claims))
	yesKeys := make([]string, len(in.Claims))
	for i, claim := range in.Claims {
		yes, opts := s.twoOptions("yes, the evidence supports this claim", "no, the evidence contradicts this claim or does not support it")
		yesKeys[i] = yes
		qs = append(qs, jev.QuestionEntry{ID: fmt.Sprintf("claim%d", i), Question: jev.Question{
			Type:         "choice",
			Instructions: map[string]any{"question": "Does the evidence support the claim?", "claim": claim},
			Criteria:     opts,
		}})
	}
	resp, err := s.provider.Predict(ctx, []jev.Request{{State: evidence, Questions: qs}})
	if err != nil {
		return nil, err
	}
	gate := s.gate.With(in.AutoAccept)
	out := &CheckOutput{Meta: metaFrom(pre, resp, s.provider.Name(), started)}
	for i, claim := range in.Claims {
		a, ok := resp[0].Answers[fmt.Sprintf("claim%d", i)]
		r := CheckResult{Claim: claim, Verdict: VerdictUnknown, Action: ActionReview, Status: StatusInvalid}
		if top := topProbability(a); ok && top >= 0 && len(a.Probabilities) == 2 {
			r.PYes = a.Probabilities[yesKeys[i]]
			r.Confidence = top
			r.Status = StatusOK
			r.Action = gate.Action(top)
			if r.PYes >= 0.5 {
				r.Verdict = VerdictSupported
			} else {
				r.Verdict = VerdictContradicted
			}
		}
		switch {
		case r.Status == StatusInvalid:
			out.Summary.Invalid++
		case r.Action == ActionReview:
			out.Summary.Review++
		case r.Verdict == VerdictSupported:
			out.Summary.Supported++
		default:
			out.Summary.Contradicted++
		}
		out.Results = append(out.Results, r)
	}
	return out, nil
}

// twoOptions builds a neutral-key two-option criteria with the yes option in
// a random position and returns the key that means yes.
func (s *Service) twoOptions(yesText, noText string) (string, jev.Options) {
	if s.rand.Intn(2) == 0 {
		return "A", jev.Options{jev.Opt("A", yesText), jev.Opt("B", noText)}
	}
	return "B", jev.Options{jev.Opt("A", noText), jev.Opt("B", yesText)}
}
