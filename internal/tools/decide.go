package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/QuBiit0/ohmylaya/internal/jev"
)

// DecideInput is the raw typed-decision call.
type DecideInput struct {
	State      any           `json:"state" jsonschema:"The content to judge: a string or a JSON object such as a ticket or record"`
	Questions  jev.Questions `json:"questions" jsonschema:"Map of question id to {type: choice|score|noul, instructions, criteria}. choice criteria: map option->description (keep under 20). score criteria: ordered list of level descriptions. noul: yes/no question"`
	AutoAccept *float64      `json:"auto_accept,omitempty" jsonschema:"Override the confidence threshold for action=auto (default from config, 0.8)"`
}

// DecideAnswer is one gated answer.
type DecideAnswer struct {
	Type          string             `json:"type"`
	Choice        string             `json:"choice,omitempty"`
	Score         float64            `json:"score,omitempty"`
	Legend        map[string]string  `json:"legend,omitempty"`
	PYes          float64            `json:"p_yes,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Confidence    *float64           `json:"confidence"`
	Action        string             `json:"action"`
	Status        string             `json:"status"`
}

// DecideOutput is the result envelope.
type DecideOutput struct {
	Answers map[string]DecideAnswer `json:"answers"`
	Meta    Meta                    `json:"meta"`
}

// Decide evaluates arbitrary typed questions against one state.
func (s *Service) Decide(ctx context.Context, in DecideInput) (*DecideOutput, error) {
	if len(in.Questions) == 0 {
		return nil, fmt.Errorf("%w: at least one question is required", ErrInput)
	}
	maxOpts := 0
	for _, q := range in.Questions {
		if q.Question.Type == "" || q.Question.Instructions == nil {
			return nil, fmt.Errorf("%w: question %q needs type and instructions", ErrInput, q.ID)
		}
		if n := optionCount(q.Question); n > maxOpts {
			maxOpts = n
		}
	}
	started := time.Now()
	pre := PreflightState(in.State, s.budget, maxOpts)
	resp, err := s.provider.Predict(ctx, []jev.Request{{State: in.State, Questions: in.Questions}})
	if err != nil {
		return nil, err
	}
	gate := s.gate.With(in.AutoAccept)
	out := &DecideOutput{Answers: map[string]DecideAnswer{}, Meta: metaFrom(pre, resp, s.provider.Name(), started)}
	for _, q := range in.Questions {
		a, ok := resp[0].Answers[q.ID]
		if !ok {
			out.Answers[q.ID] = DecideAnswer{Type: q.Question.Type, Action: ActionReview, Status: StatusInvalid}
			continue
		}
		out.Answers[q.ID] = gateAnswer(a, gate)
	}
	return out, nil
}

// gateAnswer applies the confidence contract: two-option questions gate on
// the top probability, others on the engine confidence, and anything
// malformed is marked invalid and sent to review.
func gateAnswer(a jev.Answer, gate Gate) DecideAnswer {
	d := DecideAnswer{Type: a.Type, Choice: a.Choice, Score: a.Score, Legend: a.Legend, Probabilities: a.Probabilities, Confidence: a.Confidence, Status: StatusOK}
	var value float64
	switch a.Type {
	case "noul":
		if a.Noul < 0 || a.Noul > 1 {
			return invalid(d)
		}
		d.PYes = a.Noul
		value = maxf(a.Noul, 1-a.Noul)
	case "choice":
		top := topProbability(a)
		if top < 0 || a.Choice == "" {
			return invalid(d)
		}
		if len(a.Probabilities) == 2 {
			value = top
		} else {
			if !validConfidence(a) {
				return invalid(d)
			}
			value = *a.Confidence
		}
	case "score":
		if !validConfidence(a) || topProbability(a) < 0 {
			return invalid(d)
		}
		value = *a.Confidence
	default:
		return invalid(d)
	}
	d.Action = gate.Action(value)
	return d
}

func invalid(d DecideAnswer) DecideAnswer {
	d.Status = StatusInvalid
	d.Action = ActionReview
	return d
}

func optionCount(q jev.Question) int {
	switch c := q.Criteria.(type) {
	case jev.Options:
		return len(c)
	case map[string]any:
		return len(c)
	case []string:
		return len(c)
	case []any:
		return len(c)
	}
	return 0
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
