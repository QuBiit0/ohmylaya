// Package tools implements the typed-decision tools on top of a JEV provider.
// Each tool is a plain function on Service so it can be exposed over MCP or
// the command line without duplication.
package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/QuBiit0/ohmylaya/internal/jev"
)

// ErrInput marks caller mistakes.
var ErrInput = errors.New("tools: invalid input")

// Action values.
const (
	ActionAuto   = "auto"
	ActionReview = "review"
)

// Status values.
const (
	StatusOK      = "ok"
	StatusInvalid = "invalid_response"
)

// Verdict values for check.
const (
	VerdictSupported    = "supported"
	VerdictContradicted = "contradicted"
	VerdictUnknown      = "unknown"
)

// maxOptions is where upstream measures choice accuracy collapsing.
const maxOptions = 20

// Budget is a checkpoint's token budget.
type Budget struct {
	Context       int
	HeadMaxLen    int
	CharsPerToken float64
}

// StateTokens is what remains for the state after the option budget.
func (b Budget) StateTokens() int { return b.Context - b.HeadMaxLen }

// BudgetFor returns the budget of a checkpoint variant. Characters per token
// are conservative estimates: the multilingual SentencePiece tokenizer packs
// fewer characters per token than the byte-level BPE of the English models.
func BudgetFor(variant string) Budget {
	switch variant {
	case "english":
		return Budget{Context: 512, HeadMaxLen: 192, CharsPerToken: 3.8}
	case "typed-decisions":
		return Budget{Context: 1024, HeadMaxLen: 256, CharsPerToken: 3.8}
	default:
		return Budget{Context: 1024, HeadMaxLen: 256, CharsPerToken: 3.2}
	}
}

// Preflight is the truncation estimate for one state.
type Preflight struct {
	EstimatedTokens       int      `json:"estimated_tokens"`
	StateBudget           int      `json:"state_budget_tokens"`
	Truncated             bool     `json:"truncated"`
	EstimatedDroppedChars int      `json:"estimated_dropped_chars,omitempty"`
	Warnings              []string `json:"warnings,omitempty"`
}

// PreflightState estimates whether the engine will cut the state and warns
// about option counts past the budget.
func PreflightState(state any, b Budget, optionCount int) Preflight {
	var text string
	switch v := state.(type) {
	case string:
		text = v
	default:
		if raw, err := json.Marshal(v); err == nil {
			text = string(raw)
		}
	}
	chars := len([]rune(text))
	p := Preflight{StateBudget: b.StateTokens()}
	p.EstimatedTokens = int(math.Ceil(float64(chars) / b.CharsPerToken))
	if p.EstimatedTokens > p.StateBudget {
		p.Truncated = true
		p.EstimatedDroppedChars = chars - int(float64(p.StateBudget)*b.CharsPerToken)
		p.Warnings = append(p.Warnings, fmt.Sprintf("state is about %d tokens but the %d-token budget cuts it from the end; chunk or summarise it", p.EstimatedTokens, p.StateBudget))
	}
	if optionCount > maxOptions {
		p.Warnings = append(p.Warnings, fmt.Sprintf("%d options share a fixed token budget; accuracy drops sharply past %d, split into a coarse-to-fine choice", optionCount, maxOptions))
	}
	return p
}

// Gate turns a gating value into an action.
type Gate struct{ AutoAccept float64 }

// With returns a gate with a per-call override when one is given.
func (g Gate) With(override *float64) Gate {
	if override != nil {
		return Gate{AutoAccept: *override}
	}
	return g
}

// Action returns auto when value reaches the threshold.
func (g Gate) Action(value float64) string {
	if value >= g.AutoAccept {
		return ActionAuto
	}
	return ActionReview
}

// Meta accompanies every tool result.
type Meta struct {
	Provider              string   `json:"provider"`
	Model                 string   `json:"model"`
	Truncated             bool     `json:"truncated"`
	EstimatedDroppedChars int      `json:"estimated_dropped_chars,omitempty"`
	Warnings              []string `json:"warnings,omitempty"`
	ElapsedMS             int64    `json:"elapsed_ms"`
	Partial               bool     `json:"partial,omitempty"`
}

func metaFrom(p Preflight, resp []jev.Response, provider string, started time.Time) Meta {
	m := Meta{Provider: provider, Truncated: p.Truncated, EstimatedDroppedChars: p.EstimatedDroppedChars, Warnings: p.Warnings, ElapsedMS: time.Since(started).Milliseconds()}
	for _, r := range resp {
		if r.Model != "" {
			m.Model = r.Model
			break
		}
	}
	return m
}

// Service binds the tools to a provider and a checkpoint budget.
type Service struct {
	provider jev.Provider
	budget   Budget
	gate     Gate
	rand     *rand.Rand
	reader   *Reader
}

// NewService creates the tool set. autoAccept is the default threshold.
func NewService(p jev.Provider, variant string, autoAccept float64) *Service {
	return &Service{
		provider: p,
		budget:   BudgetFor(variant),
		gate:     Gate{AutoAccept: autoAccept},
		rand:     rand.New(rand.NewSource(time.Now().UnixNano())),
		reader:   NewReader(nil),
	}
}

// SetReader replaces the reference reader, for tests or custom fetchers.
func (s *Service) SetReader(r *Reader) { s.reader = r }

// topProbability returns the highest probability in an answer, or -1 when
// the distribution is unusable.
func topProbability(a jev.Answer) float64 {
	top := -1.0
	sum := 0.0
	for _, p := range a.Probabilities {
		if math.IsNaN(p) || math.IsInf(p, 0) || p < 0 || p > 1 {
			return -1
		}
		sum += p
		if p > top {
			top = p
		}
	}
	if len(a.Probabilities) == 0 || math.Abs(sum-1) > 0.02 {
		return -1
	}
	return top
}

// validConfidence reports whether the engine confidence is usable.
func validConfidence(a jev.Answer) bool {
	if a.Confidence == nil {
		return false
	}
	c := *a.Confidence
	return !math.IsNaN(c) && !math.IsInf(c, 0) && c >= 0 && c <= 1
}
