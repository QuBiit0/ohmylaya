# Limits

Everything here comes from the upstream Laya model card or from measurements
recorded in `docs/research/`. None of it is softened.

- Zero-shot on domain decisions: base checkpoints score 0.36 on the
  typed-decisions benchmark against a 0.46 majority-class baseline. The
  0.77 belongs to `typed-decisions`, fine-tuned on that benchmark's own
  training split. Laya is a fast base to specialise, not an oracle.
- `score` is the weakest primitive (0.37 on SST-5). Prefer `choice` or
  `check`.
- Options share a fixed budget of 192 tokens (`english`) or 256 tokens
  (others). Past about 20 options each label gets three or four tokens and
  accuracy collapses (Banking77: 0.43 against Jev's 0.87).
- State is truncated from the end at about 320 tokens (`english`) or 768
  tokens (others). Tools report `meta.truncated`.
- `noul` can follow its own labels instead of the state. ohmylaya's
  `check`, `screen` and `rerank` ask two-option choices with neutral keys in
  random order instead. Position bias between the two keys measured about
  four points.
- Engine `confidence` is normalised entropy and reads near zero on any
  two-way split. ohmylaya gates two-option answers on the top probability.
- Calibration: upstream reports expected calibration error 0.47 before
  temperature fitting and 0.08 after. Until you fit on your data, treat
  `auto` as "cheap to undo" and `review` as "ask".
- `screen` false positives: the multilingual checkpoint scored the laya.cpp
  README at 0.75 to 0.81 injection probability against 0.96 for a real
  injection. `block` means inspect.
- `action.act_probability` from the engine carries no signal and is ignored.
- One engine process serves one checkpoint. No automatic language routing.
- macOS runs the CPU backend in this release; Core ML needs exported model
  buckets nobody publishes yet.
- laya.cpp releases are upstream prereleases without GPU validation in CI.
