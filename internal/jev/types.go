// Package jev defines the TypeSafe JEV request and answer schema, which
// laya.cpp implements locally and TypeSafe serves remotely, and the providers
// that speak it.
package jev

import (
	"bytes"
	"encoding/json"
)

// Question is one typed question. Criteria is a map of option to description
// for choice, an ordered list of level descriptions for score, or an optional
// {yes, no} object for noul. Use Options for choice so option order survives
// JSON encoding.
type Question struct {
	Type         string `json:"type"`
	Instructions any    `json:"instructions"`
	Criteria     any    `json:"criteria,omitempty"`
}

// Option is one choice option.
type Option struct {
	Key         string
	Description any
}

// Options encodes as a JSON object in slice order.
type Options []Option

// MarshalJSON writes the options as an object without reordering keys.
func (o Options) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, opt := range o {
		if i > 0 {
			buf.WriteByte(',')
		}
		k, err := json.Marshal(opt.Key)
		if err != nil {
			return nil, err
		}
		v, err := json.Marshal(opt.Description)
		if err != nil {
			return nil, err
		}
		buf.Write(k)
		buf.WriteByte(':')
		buf.Write(v)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// QuestionEntry pairs a caller-chosen id with its question.
type QuestionEntry struct {
	ID       string
	Question Question
}

// Questions is an ordered question map.
type Questions []QuestionEntry

// MarshalJSON writes the questions as an object in slice order.
func (qs Questions) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, q := range qs {
		if i > 0 {
			buf.WriteByte(',')
		}
		k, err := json.Marshal(q.ID)
		if err != nil {
			return nil, err
		}
		v, err := json.Marshal(q.Question)
		if err != nil {
			return nil, err
		}
		buf.Write(k)
		buf.WriteByte(':')
		buf.Write(v)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// UnmarshalJSON reads an object and keeps its key order.
func (qs *Questions) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	if _, err := dec.Token(); err != nil { // opening brace
		return err
	}
	var out Questions
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		var q Question
		if err := dec.Decode(&q); err != nil {
			return err
		}
		out = append(out, QuestionEntry{ID: tok.(string), Question: q})
	}
	*qs = out
	return nil
}

// Request is one evaluation.
type Request struct {
	ID        string    `json:"id,omitempty"`
	Model     string    `json:"model,omitempty"`
	State     any       `json:"state"`
	Questions Questions `json:"questions"`
}

// QuestionCount returns how many questions the request carries.
func (r Request) QuestionCount() int { return len(r.Questions) }

// Action carries the engine's act/escalate output. Upstream documents that
// act_probability carries no usable signal; it is kept for completeness.
type Action struct {
	ActProbability float64 `json:"act_probability"`
}

// Answer is one typed answer. Fields are populated according to Type.
type Answer struct {
	Type          string             `json:"type"`
	Confidence    *float64           `json:"confidence,omitempty"`
	Action        *Action            `json:"action,omitempty"`
	Choice        string             `json:"choice,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Score         float64            `json:"score,omitempty"`
	Legend        map[string]string  `json:"legend,omitempty"`
	Noul          float64            `json:"noul,omitempty"`
}

// Usage is the token accounting the engine reports.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Response is the answer set for one request. Provider is filled by the
// provider that produced it and is not part of the wire schema.
type Response struct {
	Model    string            `json:"model"`
	Answers  map[string]Answer `json:"answers"`
	Usage    Usage             `json:"usage"`
	Provider string            `json:"-"`
}

// ConfidenceValue returns the confidence or 0 when absent.
func (a Answer) ConfidenceValue() float64 {
	if a.Confidence == nil {
		return 0
	}
	return *a.Confidence
}
