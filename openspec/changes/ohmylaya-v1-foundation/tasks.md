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
- [x] `internal/manifestgen`: fetches upstream `SHA256SUMS` and the HF tree API,
      regenerates `manifest.json`, fails on mismatch. Moved to its own PR
      (T2c) to keep T2 under the size limit; digests for r0002 were collected
      by hand and recorded in `docs/research/engine-verification.md`.
      `go run ./internal/manifestgen -check` confirmed those hand-collected
      digests against live upstream. (2026-09-24)
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

- [x] `internal/mcpserver`: SDK wiring, schemas inferred from Go structs
      except `decide` which has an explicit schema so question and option
      order survive, lazy engine on first call (initialize never waits),
      structured tool errors with the doctor hint. Progress notifications
      deferred: the SDK handler has no progress token plumbing worth the
      complexity in v1. (2026-09-23)
- [x] `internal/app`: runtime loading and the lazy Engine that picks the
      local or hosted provider and restarts a dead sidecar once. (2026-09-23)
- [x] `ohmylaya mcp` and `ohmylaya ask <tool>` subcommands; verified end to
      end on Windows against the real engine over stdio and stdin. (2026-09-23)
- [x] Tests with the SDK's in-memory transports. (2026-09-23)

Review forecast: about 300 lines.

## T10. Config file merge library

- [x] `internal/cfgfile`: backup, atomic write, `SetJSONPath` and
      `RemoveJSONPath` that splice text so comments and trailing commas
      survive (JSON and JSONC share one path), `SetTOMLTable` and
      `RemoveTOMLTable`, unparseable refusal. (2026-09-23)
- [x] Byte-identical round-trip tests for Claude, OpenCode and Codex
      samples. (2026-09-23)

Review forecast: about 380 lines.

## T11. Agent adapters

- [x] `internal/agents`: interface, Claude Code, Codex, OpenCode (v1 and v2,
      json or jsonc), Pi (skips with a hint when `mcp.json` is absent),
      status, stale detection, backups before every write. (2026-09-23)
- [x] `ohmylaya agents` subcommand with `--json`; exits non-zero on stale
      entries. (2026-09-23)
- [x] Round-trip tests per adapter against real-shaped samples. (2026-09-23)

Review forecast: about 400 lines; split Claude+Codex (T11a) and OpenCode+Pi (T11b).

## T12. Skill

- [x] `skills/ohmylaya/SKILL.md`, `references/questions.md`,
      `references/thresholds.md` (English, body under the budget, test
      enforced). (2026-09-23)
- [x] `internal/skill`: embed, install with version stamp, installed version
      read, remove. (2026-09-23)

Review forecast: about 250 lines, mostly prose.

## T13. Install and uninstall

- [x] `internal/install` with `Resolve` (backend, model, agents; prompts
      through a `Prompter`), `Run` (downloads with skip-when-verified,
      cuBLAS extraction, config save, smoke test that stops the engine
      afterwards, skill install, registration with Pi skip), `Uninstall`.
      Tests download a fake engine from httptest and run the smoke test for
      real. Verified on the Windows dev box against the real engine.
      (2026-09-23)
- [x] `ohmylaya install` and `ohmylaya uninstall` subcommands with flags and
      a line prompter for terminals; `ohmylaya agents --register/--unregister`
      as the per-agent toggle. (2026-09-23)
- [x] `install.sh` and `install.ps1` with checksum verification. Shell tests
      in CI deferred until the first release publishes real archives.
      (2026-09-23)

Review forecast: about 400 lines; scripts may be a separate PR (T13b).

## T14. Doctor

- [x] `internal/doctor`: check catalogue (platform, home, engine,
      runtime-deps, model, port, sidecar, gpu, smoke, skill per agent,
      agents, version), human and JSON output, `--smoke` that stops an
      engine it started, `--fail-on warn`, three-second offline skip.
      Verified on the Windows dev box. (2026-09-23)

Review forecast: about 300 lines.

## T15. Update

- [x] `internal/update`: GitHub release lookup, archive download and
      checksum verify, swap per OS with `.new`/`.old` staging and
      `SwapPending` on start, post-update reconcile by running the new
      binary's `install`, `--check`. Engine and model reconcile happen
      through `install`, which skips verified files; the `--engine` and
      `--model` flags and engine rollback on failed smoke are folded into
      install's smoke test (a failed smoke leaves the previous config).
      (2026-09-23)

Review forecast: about 350 lines.

## T16. TUI

- [x] `internal/tui`: root model with injected Services, adaptive theme,
      Status, Setup, Agents, Update, Doctor, Logs; keys ignored while busy;
      no-argument launch on a terminal, plain status on pipes or
      `TERM=dumb`. Every action calls the same code as the subcommands.
      (2026-09-23)
- [x] Tests drive Update and View directly with fake services instead of
      `teatest`, which would add a dependency for little gain. (2026-09-23)

Review forecast: about 400 lines per PR, expected three PRs (shell and Status;
Setup and Agents; Update, Doctor and Logs).

## T17. Docs and release

- [x] README (honest limits first, one-shot commands, agent table),
      `docs/usage.md`, `docs/limits.md`, `docs/troubleshooting.md`; the
      agent table lives in the README. (2026-09-23)
- [x] Release workflow on `v*` tags running tests then GoReleaser.
      (2026-09-23)
- [ ] Tag v0.1.0, publish release, verify the one-shot on all three OSes.
      Windows verified from source; Linux and macOS need a machine.

Review forecast: docs only.

## T18. Token and quality benchmark

- [x] `bench/` harness that replays hand-labelled tool calls (fetch, file
      search, issue triage, claim check) and records estimated tokens with
      and without the tool, engine time and decision accuracy.
- [ ] Replay full agent sessions with a frontier model and record real input
      and output tokens (moved to v2).
- [x] Publish `docs/benchmarks.md` with the measured numbers and the corpus.
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
