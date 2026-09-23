# Tasks: ohmylaya v1 foundation

Each task is one reviewable work unit: tests, code and docs together, under
400 changed lines, one pull request. Order matters; later tasks build on
earlier packages. Strict TDD applies to every Go task.

## T0. Engine verification spike (no code merged)

- [x] Download laya.cpp r0002 Windows Vulkan executable and the multilingual
      checkpoint; verify digests. (2026-09-23)
- [x] Verify: Vulkan executable serves with `--cpu` and with `--vulkan` on
      Windows; record `/health`, `/predict`, `/v1/systemone` responses into
      `testdata/engine/r0002-multilingual/`. (2026-09-23)
- [x] Measure cold start and latency on CPU and Vulkan (Windows). (2026-09-23)
- [x] Write `docs/research/engine-verification.md`; design D3 and the
      confidence contract updated. (2026-09-23)
- [ ] Verify Vulkan executable with `--cpu` on a host without a Vulkan loader
      (Windows and Linux).
- [ ] Verify macOS executable honours `--cpu`.
- [ ] Measure CUDA on Windows (needs the two cuBLAS DLLs).

Review forecast: docs only, about 150 lines.

## T1. Repository skeleton and CI

- [x] `go.mod` (`github.com/QuBiit0/ohmylaya`), `cmd/ohmylaya/main.go`,
      `internal/cli` with `version` and `help`, `internal/buildinfo`,
      `Makefile`. Tests written first. (2026-09-23)
- [x] GitHub Actions: vet, staticcheck, `go test -race` on three OSes, PR
      size check. (2026-09-23)
- [x] `.goreleaser.yaml` for windows/amd64, linux/amd64, linux/arm64,
      darwin/arm64 with `checksums.txt`. (2026-09-23)
- [x] CONTRIBUTING.md, AI_POLICY.md, issue and PR templates. (2026-09-23)

Review forecast: about 300 lines, mostly config.

## T2. Manifest and platform probes

- [x] `internal/manifest`: types, embedded `manifest.json` for r0002 and the
      pinned Hugging Face revision, loader with validation tests. (2026-09-23)
- [ ] `internal/manifestgen`: fetches upstream `SHA256SUMS` and the HF tree API,
      regenerates `manifest.json`, fails on mismatch. Moved to its own PR
      (T2c) to keep T2 under the size limit; digests for r0002 were collected
      by hand and recorded in `docs/research/engine-verification.md`.
- [x] `internal/platform`: OS/arch, glibc version, library probes
      (`vulkan-1.dll`, `nvcuda.dll`, `libvulkan.so.1`, `libcuda.so.1`),
      recommendation table from the installer spec, with fakeable probes.
      Verified on the Windows dev box: vulkan, cuda, cpu detected. (2026-09-23)

Review forecast: about 380 lines.

## T3. Acquire

- [x] `internal/acquire`: HTTP downloader with range resume, progress callback,
      SHA-256 verification, atomic placement, zip member extraction for cuBLAS.
      (2026-09-23)
- [x] Tests with `httptest` servers simulating interruption, servers that
      ignore Range, corruption, cancellation and already-present files.
      (2026-09-23)

Review forecast: about 350 lines.

## T4. Config and home layout

- [x] `internal/config`: Home, Layout, defaults, TOML load and save with
      go-toml/v2, env overrides, validation. State helpers move to
      `internal/sidecar` (T6) where they are used. (2026-09-23)
- [x] Golden test for the default `config.toml`. (2026-09-23)

Review forecast: about 250 lines.

## T5. JEV types and providers

- [x] `internal/jev`: request and answer types with order-preserving
      `Questions` and `Options`, `Provider` interface, `local` provider with
      `/predict` batching, question-limit splitting and merge, 503 backoff,
      422 mapping and `/health`; `typesafe` provider with bearer auth and
      429/529 backoff. (2026-09-23)
- [x] Golden tests using recorded engine responses from T0, plus an
      `engine`-tagged test run against the live engine (12 questions split
      across calls and merged). (2026-09-23)

Review forecast: about 380 lines.

## T6. Sidecar supervision

- [x] `internal/sidecar`: lock, state file, detached spawn per OS, readiness
      wait, client registration, idle reaper with activity touch, log
      rotation at spawn, port fallback, config-change respawn. Overload
      retry lives in `internal/jev`. Crash restart limit deferred to the MCP
      layer (T9) which calls Ensure again once. (2026-09-23)
- [x] Fake engine binary built in TestMain; tests for spawn then attach,
      stale lock, crash with log tail, port fallback, stop, idle reaper,
      touch, log rotation. (2026-09-23)

Review forecast: about 400 lines; may split spawn (T6a) and lifecycle (T6b).

## T7. Tools: preflight, decide, check

- [x] `internal/tools`: preflight estimator, confidence contract, `decide`,
      `check` with the two-option workaround and key randomisation, reference
      reader (path, url with HTML to text, glob with `**`). (2026-09-23)
- [x] Tests with fake and scripted providers. (2026-09-23)

Review forecast: about 350 lines.

## T8. Tools: classify, screen, rerank

- [x] `classify` (items, paths or glob; ordered results; excerpts), `screen`
      (three two-option questions, block/skip/allow, fail closed, text only
      on allow with include_text), `rerank` (batches of 8, sort, top_k,
      partial on deadline). (2026-09-23)
- [x] Tests with scripted providers, httptest for URL refs. (2026-09-23)

Review forecast: about 350 lines.

## T9. MCP server and `ask` subcommand

- [ ] `internal/mcpserver`: SDK wiring, tool schemas from Go structs, fast
      initialize, background `Ensure`, structured errors with doctor hint,
      progress notifications.
- [ ] `ohmylaya mcp` and `ohmylaya ask <tool>` subcommands.
- [ ] Test with the SDK's in-memory client transport.

Review forecast: about 300 lines.

## T10. Config file merge library

- [ ] `internal/cfgfile`: backup, atomic write, `MergeJSON`, `MergeJSONC`,
      `MergeTOML`, `Remove*` counterparts, unparseable refusal.
- [ ] Golden round-trip tests.

Review forecast: about 380 lines.

## T11. Agent adapters

- [ ] `internal/agents`: interface, Claude Code, Codex, OpenCode (v1 and v2),
      Pi (with missing `mcp.json` behaviour), status table, stale detection.
- [ ] `ohmylaya agents` subcommand.
- [ ] Golden tests per adapter.

Review forecast: about 400 lines; split Claude+Codex (T11a) and OpenCode+Pi (T11b).

## T12. Skill

- [ ] `skills/ohmylaya/SKILL.md`, `references/questions.md`,
      `references/thresholds.md` (English, under the token budget).
- [ ] `internal/skill`: embed, install per agent, version check.

Review forecast: about 250 lines, mostly prose.

## T13. Install and uninstall

- [ ] `ohmylaya install` interactive and flag-driven flow composing T2 to T12,
      summary with sizes, smoke test, idempotent re-run report.
- [ ] `ohmylaya uninstall` with backup restore and `--keep-models`.
- [ ] `install.sh` and `install.ps1` with checksum verification; shell tests
      via `bats` and Pester in CI.

Review forecast: about 400 lines; scripts may be a separate PR (T13b).

## T14. Doctor

- [ ] `internal/doctor`: check catalogue, human and JSON output, `--smoke`,
      `--fail-on`, offline timeouts.

Review forecast: about 300 lines.

## T15. Update

- [ ] `internal/update`: GitHub release lookup, archive download and verify,
      atomic swap per OS, post-update reconcile, `--check`, `--engine`,
      `--model`, engine rollback on failed smoke.

Review forecast: about 350 lines.

## T16. TUI

- [ ] `internal/tui`: root model, theme, Status, Setup, Agents, Update,
      Doctor, Logs; non-interactive fallback.
- [ ] `teatest` golden snapshots per screen.

Review forecast: about 400 lines per PR, expected three PRs (shell and Status;
Setup and Agents; Update, Doctor and Logs).

## T17. Docs and release

- [ ] README (honest limits first, one-shot commands, agent table),
      `docs/usage.md`, `docs/limits.md`, `docs/agents.md`, `docs/troubleshooting.md`.
- [ ] Tag v0.1.0, publish release, verify one-shot on all three OSes.

Review forecast: docs only.

## T18. Token and quality benchmark

- [ ] `bench/` harness that replays recorded agent sessions (fetch, file
      search, issue triage, claim check) with and without ohmylaya tools and
      records model input and output tokens, wall time and decision accuracy
      against hand labels.
- [ ] Publish `docs/benchmarks.md` with the measured numbers and the corpus.
      No token or accuracy claim enters the README before this task lands.

Review forecast: about 350 lines plus fixtures.

## Deferred (v2 candidates)

- Apple Core ML with published compiled buckets.
- Calibration store and temperature fitting from user-labelled outcomes.
- Multi-process routing across checkpoints by language.
- Optional gentle-ai subagent and prompt hooks (screen in research, classify
  in triage, check in verify) once the tools have measured precision.
- Homebrew tap, Scoop bucket, winget manifest.
- minisign release signatures.
