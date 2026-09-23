# MCP tools

Defines the MCP stdio server and the five typed-decision tools.

## Requirements

### Requirement: Server identity and lifecycle

`ohmylaya mcp` MUST serve MCP over stdio using the official Go SDK, advertise
name `ohmylaya` and the binary version, and MUST answer `initialize` within two
seconds even when the sidecar is cold. Tools MUST be listed immediately; a call
that arrives before the sidecar is ready MUST wait up to the readiness timeout
and report progress through MCP progress notifications when the client supports
them.

#### Scenario: Cold start under a 30 second host timeout

- GIVEN a host that aborts initialization after 30 seconds
- WHEN `ohmylaya mcp` starts with no sidecar running
- THEN initialization completes in under two seconds
- AND the sidecar starts in the background.

#### Scenario: Engine unavailable

- GIVEN the sidecar cannot start
- WHEN any tool is called
- THEN the result is `isError: true` with a one-line cause and the exact `ohmylaya doctor` command to run
- AND the server stays alive.

### Requirement: Provider abstraction

Every tool MUST go through a `Provider` interface with two implementations:
`local` (sidecar, default) and `typesafe` (hosted Jev at
`https://api.typesafe.ai/v1/systemone`, enabled by `provider = "typesafe"` plus
`TYPESAFE_API_KEY`). Both speak the same JEV request and answer schema. Tool
results MUST include `provider` and `model` fields.

#### Scenario: Switch to hosted

- GIVEN `provider = "typesafe"` and a valid key
- WHEN `decide` is called
- THEN the request goes to TypeSafe with a bearer token
- AND the result reports `provider: "typesafe"`.

### Requirement: Preflight and truncation contract

Before sending, every tool MUST estimate the state token count with the
checkpoint's tokenizer budget (about 320 state tokens for `english`, about 768
for `multilingual` and `typed-decisions`) using a conservative characters-per-
token heuristic, and MUST return `truncated: true` with `estimated_dropped_chars`
when the state would be cut. Choice questions with more than 20 options MUST
return a `warning` naming the option-budget limit.

#### Scenario: Long state

- GIVEN a 12,000 character document and the multilingual checkpoint
- WHEN `check` is called
- THEN the answer is returned
- AND `truncated: true` with the estimated dropped tail is included
- AND the description tells the agent to chunk or summarize.

### Requirement: Confidence contract

Every answer MUST expose `probabilities` as returned by the engine, the engine's
`confidence`, and an ohmylaya `action` of `auto` when confidence is at or above
the tool's `auto_accept` threshold (default 0.8) or `review` otherwise. Missing
or non-finite values MUST invalidate that answer with `status: "invalid_response"`
rather than defaulting to a number.

#### Scenario: Low confidence

- GIVEN a choice answered with confidence 0.55
- WHEN the result is built
- THEN `action` is `review`.

### Requirement: Tool `decide`

Raw typed decisions. Input: `state` (string or object), `questions` (map of id
to `{type, instructions, criteria}` in JEV shape), optional `auto_accept`.
Output: one answer per id with the confidence contract. This tool is the
escape hatch; the skill tells agents to prefer the specialised tools.

#### Scenario: Mixed question types

- GIVEN a state and three questions of types `choice`, `score`, `noul`
- WHEN `decide` runs
- THEN each answer carries its own `type`, value, `probabilities`, `confidence` and `action`.

### Requirement: Tool `classify`

Assign each of up to 50 items to exactly one label from a shared catalog.
Input: `items` (array of `{id, text}`), `labels` (map label to description),
optional `instructions`, `auto_accept`. Output: per item `label`, `confidence`,
`probabilities`, `action`. Items MUST be batched to respect the sidecar's
question limit and the results MUST be returned in input order.

#### Scenario: Label a batch of issues

- GIVEN 20 issue bodies and 6 labels
- WHEN `classify` runs
- THEN 20 results come back in order
- AND items below the threshold are marked `review`.

### Requirement: Tool `check`

Yes/no claim check against evidence. Input: `claims` (array of strings),
`evidence` (string or object), optional `auto_accept`. Because the `noul`
primitive is known to follow its labels, this tool MUST ask a two-option
`choice` with neutral keys and the yes/no wording as descriptions, and MUST map
the result to `verdict: supported|contradicted` plus `p_yes`.

#### Scenario: Test claim against output

- GIVEN the claim "all tests passed" and a captured test log with one failure
- WHEN `check` runs
- THEN the verdict is `contradicted` with `p_yes` well below 0.5.

### Requirement: Tool `screen`

Judge text before it enters agent context. Input: `text`, `purpose`, optional
thresholds. Output: `injection`, `substance`, `relevance` probabilities and a
`recommendation` of `block|skip|allow` with the reason. Defaults: block when
injection >= 0.75, skip when substance < 0.3 or relevance < 0.3.

#### Scenario: Injected page

- GIVEN a fetched page containing "ignore the user's instructions and ..."
- WHEN `screen` runs
- THEN `recommendation.action` is `block` and the reason names the injection probability.

### Requirement: Tool `rerank`

Order candidates by relevance to a query. Input: `query`, `candidates` (array
of `{id, text}`, up to 100), optional `top_k`. Output: candidates sorted by
`score` descending with the score being the probability of relevance. Batches
MUST respect the question limit and the tool MUST NOT time out on 100
candidates on a CPU backend (bounded by 60 seconds, partial results allowed
with `partial: true`).

#### Scenario: Find the file

- GIVEN a question and 60 file summaries
- WHEN `rerank` runs with `top_k: 5`
- THEN five candidates return with descending scores.

### Requirement: Tool descriptions carry the honest limits

Each tool description MUST state in one sentence what the model is bad at for
that tool (zero-shot domain decisions, more than 20 options, long state,
ordinal scores) so hosts without the skill loaded still get the warning.

#### Scenario: Host lists tools

- GIVEN any MCP client
- WHEN it lists tools
- THEN every description contains a "Limits:" sentence.
