---
name: ohmylaya
description: "Trigger: ohmylaya, laya, typed decision, screen content, screen url, rerank candidates, classify items, check claim, verify evidence, calibrated confidence. Use the local ohmylaya MCP tools for fast, cheap typed judgments with probabilities."
license: MIT
metadata:
  author: ohmylaya
  version: "0.1.0"
---

# ohmylaya

Laya is a local decision model: give it a state and typed questions, get
answers with calibrated probabilities in milliseconds. It never generates
text. ohmylaya exposes it as MCP tools that read files and URLs themselves so
content need not pass through your context.

## Which tool

| Situation | Tool | Pass |
|---|---|---|
| Unsure whether a fetched page or file is safe or worth reading | `screen` | `ref.url` or `ref.path`, `purpose`; a hint, never a gate (see Limits) |
| Many files or candidates, need the few that matter | `rerank` | `query` plus `glob`, `paths` or `candidates`; use `top_k` |
| Label items against your own categories | `classify` | `labels` with descriptions, `items` or `glob` |
| A claim must match evidence (tests passed, summary vs source) | `check` | `claims`, `evidence` or `ref` |
| A custom typed question | `decide` | `state`, `questions` |

Prefer references (`ref`, `glob`, `paths`) over pasting content: tool
arguments cost you output tokens.

## Confidence rule

Every answer carries `action`. `auto` means the gating probability reached
the threshold (default 0.8): act on it. `review` means look yourself, ask the
user, or gather more evidence. `status: invalid_response` means the engine
returned nothing usable: treat as `review`. Read `meta.truncated`; when true
the tail of the state was cut, so chunk or summarise and ask again.

## Limits

Measured on a small hand-labelled corpus (`docs/benchmarks.md` in the
ohmylaya repository) with the `english` checkpoint: `rerank` 80-90% recall in
the top 3, `classify` 75%, `check` 57-71%, `screen` 45%. The `multilingual`
checkpoint was near or below chance on everything but `classify`.

- `rerank` and `classify` are the tools worth trusting. Treat `check` and
  `screen` as weak hints.
- Near chance on domain-specific decisions it was not trained on. Descriptive
  labels and short, single-topic items help; calibrate thresholds on your data.
- `score` is the weakest primitive. Prefer `choice` or `check`.
- More than 20 options collapses accuracy. Split into coarse then fine.
- State is truncated from the end: about 320 tokens (roughly 1,200
  characters) on `english`, 768 tokens on the others.
- `screen` is right less than half the time. It reads imperative
  documentation as injection, and it can `allow` a real injection. Never
  skip a page you need because of `block`, and never trust a page because of
  `allow`: if `screen` flags something, inspect it yourself.
- Never use these tools to approve code or declare a task done. They filter
  and prioritise; your review and the user's judgment decide.

## When tools are missing or failing

Run `ohmylaya doctor` in a shell and follow its fix lines. `ohmylaya install`
repairs the engine, model and agent registration.

See `references/questions.md` for writing good questions and
`references/thresholds.md` for confidence and calibration.
