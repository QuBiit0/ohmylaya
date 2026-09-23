// Package mcpserver exposes the tools over the Model Context Protocol.
package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/QuBiit0/ohmylaya/internal/jev"
	"github.com/QuBiit0/ohmylaya/internal/tools"
)

// Engine supplies a ready tool service, starting the sidecar when needed.
type Engine interface {
	Service(ctx context.Context) (*tools.Service, error)
	// Touch records activity so the idle reaper keeps the engine alive.
	Touch()
}

// DoctorHint is appended to every engine failure.
const DoctorHint = "Run `ohmylaya doctor` to diagnose, or `ohmylaya install` to repair."

// Descriptions carry the honest limits so hosts without the skill still see them.
var descriptions = map[string]string{
	"decide": "Ask typed questions (choice, score, noul) about a state and get calibrated probabilities. Prefer the specialised tools; use this for custom questions. " +
		"Limits: base checkpoints are near chance on domain-specific zero-shot decisions, score is the weakest primitive, keep under 20 options, long state is truncated from the end (see meta.truncated).",
	"classify": "Assign each item to one label from your own catalogue, in batches of up to 50, with a probability per label and an auto/review action. Accepts items, paths or a glob so file contents never pass through your context. " +
		"Limits: describe labels, keep under 20, and calibrate the threshold on your data; near chance on unfamiliar domains without descriptions.",
	"check": "Verify claims against evidence (inline, path or url): returns supported/contradicted with p_yes and an auto/review action per claim. Good for 'tests passed' against a log or a summary against its source. " +
		"Limits: evidence longer than about 2,500 characters is truncated from the end; it judges support, not truth.",
	"screen": "Judge text or a url/path before it enters your context: probability of prompt injection, substance and relevance to your purpose, with a block/skip/allow recommendation. Blocked or skipped content is never returned. " +
		"Limits: long pages are judged on their first ~2,500 characters; the base model over-reads imperative documentation as injection, so treat block as a strong signal to verify, skip as advisory, and calibrate thresholds on your own pages.",
	"rerank": "Order up to 100 candidates (inline, paths or glob) by relevance to a query and return the top k with excerpts, without embeddings or an index. " +
		"Limits: each candidate is judged on its first ~2,500 characters; scores are relative, not calibrated across queries.",
}

// New builds the MCP server with the five tools.
func New(engine Engine, version string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "ohmylaya", Version: version}, nil)

	mcp.AddTool(s, &mcp.Tool{Name: "decide", Description: descriptions["decide"], InputSchema: decideSchema()},
		wrap(engine, func(ctx context.Context, svc *tools.Service, in decideArgs) (any, error) {
			return svc.Decide(ctx, tools.DecideInput{State: in.State, Questions: in.Questions, AutoAccept: in.AutoAccept})
		}))
	mcp.AddTool(s, &mcp.Tool{Name: "classify", Description: descriptions["classify"]},
		wrap(engine, func(ctx context.Context, svc *tools.Service, in tools.ClassifyInput) (any, error) {
			return svc.Classify(ctx, in)
		}))
	mcp.AddTool(s, &mcp.Tool{Name: "check", Description: descriptions["check"]},
		wrap(engine, func(ctx context.Context, svc *tools.Service, in tools.CheckInput) (any, error) {
			return svc.Check(ctx, in)
		}))
	mcp.AddTool(s, &mcp.Tool{Name: "screen", Description: descriptions["screen"]},
		wrap(engine, func(ctx context.Context, svc *tools.Service, in tools.ScreenInput) (any, error) {
			return svc.Screen(ctx, in)
		}))
	mcp.AddTool(s, &mcp.Tool{Name: "rerank", Description: descriptions["rerank"]},
		wrap(engine, func(ctx context.Context, svc *tools.Service, in tools.RerankInput) (any, error) {
			return svc.Rerank(ctx, in)
		}))
	return s
}

// Run serves over stdio until the client disconnects.
func Run(ctx context.Context, engine Engine, version string) error {
	return New(engine, version).Run(ctx, &mcp.StdioTransport{})
}

// decideArgs is the MCP-facing decide input; Questions keeps object order.
type decideArgs struct {
	State      any           `json:"state"`
	Questions  jev.Questions `json:"questions"`
	AutoAccept *float64      `json:"auto_accept,omitempty"`
}

func decideSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:     "object",
		Required: []string{"state", "questions"},
		Properties: map[string]*jsonschema.Schema{
			"state": {Description: "The content to judge: a string or a JSON object such as a ticket or record"},
			"questions": {
				Type:        "object",
				Description: "Map of question id to {type: choice|score|noul, instructions, criteria}. choice criteria: map option->description (keep under 20). score criteria: ordered list of level descriptions. noul: yes/no question",
				AdditionalProperties: &jsonschema.Schema{
					Type:     "object",
					Required: []string{"type", "instructions"},
					Properties: map[string]*jsonschema.Schema{
						"type":         {Type: "string", Enum: []any{"choice", "score", "noul"}},
						"instructions": {Description: "The question, as a string or an object with the question and data it refers to"},
						"criteria":     {Description: "choice: object option->description; score: array of level descriptions; noul: optional {yes, no} descriptions"},
					},
				},
			},
			"auto_accept": {Type: "number", Minimum: ptr(0.0), Maximum: ptr(1.0), Description: "Override the confidence threshold for action=auto (default 0.8)"},
		},
	}
}

func ptr(f float64) *float64 { return &f }

// wrap adapts a tool function to the SDK handler, turning every failure into
// a tool error the agent can read.
func wrap[In any](engine Engine, fn func(context.Context, *tools.Service, In) (any, error)) mcp.ToolHandlerFor[In, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		svc, err := engine.Service(ctx)
		if err != nil {
			return errResult(fmt.Sprintf("engine unavailable: %v. %s", err, DoctorHint)), nil, nil
		}
		engine.Touch()
		out, err := fn(ctx, svc, in)
		if err != nil {
			return errResult(explain(err)), nil, nil
		}
		b, err := json.Marshal(out)
		if err != nil {
			return errResult("encode result: " + err.Error()), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil, nil
	}
}

func explain(err error) string {
	switch {
	case errors.Is(err, tools.ErrInput):
		return err.Error()
	case errors.Is(err, jev.ErrOverloaded):
		return err.Error() + ". The engine queue is full; retry shortly or raise batch.max_pending in config.toml."
	case errors.Is(err, jev.ErrInvalidRequest):
		return err.Error() + ". The engine rejected the request; check question types and criteria."
	case errors.Is(err, jev.ErrUnauthorized):
		return err.Error() + ". Set TYPESAFE_API_KEY for the hosted provider."
	}
	return err.Error() + ". " + DoctorHint
}

func errResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: msg}}}
}
