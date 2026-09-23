# Writing questions Laya answers well

Laya scores each option at its own marker inside a fixed token budget, so
wording is most of the accuracy. These patterns come from the upstream model
card and from measurements recorded in `docs/research/`.

## General

- One topic per question. Split compound questions.
- Put the data the question refers to inside the state, and keep the
  instructions short. Structured instructions work: `{"question": "...",
  "claim": "..."}`.
- Describe options. `{"billing": "invoices, payments, refunds"}` beats
  `{"billing": null}`.
- Keep the state under about 2,500 characters. The engine truncates from the
  end, and the answer is usually at the end.

## screen

```json
{"ref": {"url": "https://example.com/pricing"}, "purpose": "extract the pricing tiers"}
```

`purpose` drives relevance. Be concrete: "find the install command for
Windows" rather than "research".

## rerank

```json
{"query": "where is the sidecar port chosen?", "glob": "internal/**/*.go", "top_k": 5}
```

The query should read like the question you would ask a colleague. File
contents are judged on their first part, so large files rank by their header.

## classify

```json
{
  "labels": [
    {"key": "bug", "description": "something that worked before is broken"},
    {"key": "feature", "description": "a request for new behaviour"},
    {"key": "question", "description": "asking how to do something"}
  ],
  "items": [{"id": "#12", "text": "Crash on startup after update"}]
}
```

Fewer, well-described labels win. Above 20 labels accuracy collapses; group
first, then refine.

## check

```json
{
  "claims": ["all tests passed", "the change touched only internal/tools"],
  "ref": {"path": "/tmp/test-output.log"}
}
```

Each claim is asked as a two-option choice with neutral keys in random
order, because the yes/no primitive follows its labels. `p_yes` is the
probability the evidence supports the claim.

## decide

Use `choice` for categories, `noul` for yes/no when the state clearly
answers it, and `score` only when you need an ordinal expectation and can
tolerate weaker accuracy.

```json
{
  "state": {"subject": "Duplicate invoice", "body": "Charged twice, refund today or I cancel."},
  "questions": {
    "department": {"type": "choice", "instructions": "Which department should handle this?", "criteria": {"billing": "payments and refunds", "technical": "bugs and outages"}},
    "churn": {"type": "noul", "instructions": "Does the customer threaten to leave?"}
  }
}
```
