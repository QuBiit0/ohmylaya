# Benchmarks

What ohmylaya's tools get right, and how many model tokens they save, on a
small hand-labelled corpus. Measured on 2026-09-24.

## Summary

- **The checkpoint matters more than anything else.** On this mostly English
  corpus, `english` beats `multilingual` on every tool. The largest gap is
  `rerank`: 80% against 0 to 10%.
- **Only `classify` and `rerank` on `english` are clearly useful.** `check`
  and `screen` are near or below chance on both checkpoints.
- **Token savings are real but conditional.** `rerank`, `check` and `screen`
  cut what the agent reads by 23% to 97%. A saving is worth something only
  when the verdict is right. `classify` on short items costs more tokens than
  reading them.

## Results

Windows 11, Vulkan, laya.cpp r0002, ohmylaya at commit `665553d`. Each
checkpoint ran the corpus three or more times. Where the runs differed, the
table gives the range.

| Tool | Decisions | Chance | multilingual | english | Tokens without | Tokens with | Saved | Median ms (english) |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| `check` | 14 | 50% | 36–50% | 57–71% | 2,950 | 1,030 | 65% | 270–640 |
| `classify` | 16 | 25% | 69% | 75% | 707 | 2,006 | −183% | 260 |
| `rerank` (top 3 of 20) | 10 | 15% | 0–10% | 80% | 314,820 | 9,520 | 97% | 3,000 |
| `screen` | 11 | 33% | 0–9% | 45% | 1,442 | 1,103 | 24% | 225 |

- **Chance** is the accuracy of a uniform random answer.
- **Tokens without** is everything the agent would read to decide on its own.
- **Tokens with** is the tool call plus its compact JSON answer.
- Token counts barely depend on the checkpoint, so the table shows the
  `english` runs.
- Medians on `multilingual` were lower: check 140 ms, classify 125 ms,
  rerank 4.6 s, screen 115 ms.
- The first call after an engine start pays a shader warmup. That shows as
  the 640 ms upper bound for `check`.

### What went wrong on `multilingual`

- **`screen` marks almost everything as prompt injection.** The injection
  probability is 0.8 to 1.0 for a Go documentation page, an nginx guide, a
  cookie wall and a 404 page. Relevance is about 0.9 for pages unrelated to
  the purpose.
- **`rerank` ranked the right file at a median position of 9 out of 20.**
- **`check` answered "all tests passed" as supported on a failing log**, and
  as contradicted on a passing one.

## Method

`go run ./bench -bin <ohmylaya>` replays every suite in `testdata/bench/`
through `ohmylaya ask <tool>`. It uses whatever engine and checkpoint the
current `OHMYLAYA_HOME` has installed.

Each case scores one or more decisions against hand labels:

| Tool | Scenario | Label | Scored as |
|---|---|---|---|
| `screen` | Reading a fetched page | `allow`, `skip` or `block` | Exact match of `recommendation.action` |
| `classify` | Issue triage into bug, feature, question or docs | Label per issue | Exact match per item |
| `check` | Claims about CI, audit, build and deploy logs | `supported` or `contradicted` per claim | Exact match per claim |
| `rerank` | Finding the file that answers a question, among 20 real files of this repository | The one relevant path | Recall in the top 3 |

Tokens are estimated as one per four characters. Both columns use the same
estimate, so the ratio is meaningful even where the absolute numbers are not.

## Corpus

51 labelled decisions, written by the maintainers:

- 11 pages, including three prompt injections of different subtlety and one
  Spanish page.
- 16 issues.
- 6 logs.
- 10 file-search questions.

The texts are in `testdata/bench/`.

The `failure-at-end` log puts its failing test past the 768-token state
budget on purpose. Both checkpoints miss it because the engine never sees
it. `rerank` has the same limit on whole files: it sees only the package
comment and the imports of long files.

## Limits

- **The corpus is small and ours.** Treat the numbers as a smoke test of the
  tools, not a property of Laya. A difference of one decision moves `rerank`
  and `screen` by about 10 points.
- **Results are not fully deterministic on Vulkan.** Borderline decisions
  flip between runs, which is why the table shows ranges.
- **The `rerank` labels track the current code.** The corpus names real
  files of this repository, so refactors can move the answer.
- **Only the offline mode exists.** No agent sessions are replayed end to
  end, so the benchmark says nothing about final answer quality.

## Reproduce

```sh
go build -o bin/ohmylaya ./cmd/ohmylaya
bin/ohmylaya install --model english --agents none --yes
go run ./bench -bin bin/ohmylaya
```

To compare checkpoints without touching an existing installation, point
`OHMYLAYA_HOME` and `OHMYLAYA_PORT` at a scratch directory and a free port
before installing.
