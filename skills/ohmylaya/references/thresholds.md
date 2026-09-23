# Confidence, thresholds and calibration

## What the numbers are

- `probabilities`: the engine's distribution over options after its built-in
  temperature. They sum to one.
- `confidence` on multi-option answers: normalised entropy of that
  distribution. It is low whenever probability is spread, even when the top
  option is right, and it reads near zero on any two-way split. That is why
  ohmylaya gates two-option questions (`check`, `screen`, `rerank`) on the top
  probability instead.
- `p_yes` and `score` on `rerank`: probability of the yes option.
- `action`: `auto` when the gating value reaches `auto_accept`, else
  `review`.

## Choosing auto_accept

The default 0.8 is a starting point, not a measurement. Laya ships
over-confident until a temperature is fitted on your own labelled examples:
upstream reports calibration error dropping from 0.47 to 0.08 after fitting.
Until you have done that:

- Use `auto` answers for actions that are cheap to undo: skipping a page,
  ordering a list, proposing a label.
- Send `review` answers, and anything irreversible, to a human or to a
  stronger model.
- Keep a log of decisions and outcomes. Fifty labelled examples per question
  type are enough to pick a threshold that matches the precision you need.

## Position bias

The same claim scored 0.82 with yes on key A and 0.86 with yes on key B in
testing. ohmylaya randomises the yes key per claim so the bias averages out
instead of locking in. Expect a few points of noise between identical calls.

## screen thresholds

Defaults: block when injection >= 0.75, skip when substance < 0.3 or
relevance < 0.3. Imperative documentation (build instructions, READMEs)
scores around 0.8 on injection with the base checkpoint, so `block` on a
page you expected to be safe means "inspect", not "discard". Pass
`thresholds` to tune per call once you have measured your own pages.
