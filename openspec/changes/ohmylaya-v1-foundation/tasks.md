# Tasks: ohmylaya v1 foundation

Each task is one reviewable work unit: tests, code and docs together, under
400 changed lines, one pull request. Order matters; later tasks build on
earlier packages. Strict TDD applies to every Go task.

## T0. Engine verification spike (no code merged)

- [ ] Download laya.cpp r0002 Vulkan executables for Windows and Linux and the
      macOS Core ML executable; download the multilingual checkpoint.
- [ ] Verify: Vulkan executable starts with `--cpu` on a machine without a
      Vulkan loader (Windows and Linux).
- [ ] Verify: macOS executable honours `--cpu`.
- [ ] Verify: `/health`, `/predict`, `/v1/systemone` shapes against the smoke
      corpus; record responses into `testdata/engine/` for golden tests.
- [ ] Measure cold start and per-question latency on CPU, Vulkan, CUDA.
- [ ] Write `docs/research/engine-verification.md` with results and update
      design D3 if a fallback assumption is wrong.

Review forecast: docs only, about 150 lines.

## T1. Repository skeleton and CI

- [ ] `go.mod` (module path per open question 1), `cmd/ohmylaya/main.go` with
      version flag only, `Makefile` or `Taskfile`, `.golangci.yml`.
- [ ] GitHub Actions: vet, staticcheck, `go test -race` on three OSes.
- [ ] `.goreleaser.yaml` for windows/amd64, linux/amd64, linux/arm64,
      darwin/arm64 with `checksums.txt`.
- [ ] CONTRIBUTING.md, AI_POLICY.md, issue and PR templates, PR size check.

Review forecast: about 300 lines, mostly config.

## T2. Manifest and platform probes

- [ ] `internal/manifest`: types, embedded `manifest.json` for r0002 and the
      pinned Hugging Face revision, loader with validation tests.
- [ ] `internal/manifestgen`: fetches upstream `SHA256SUMS` and the HF tree API,
      regenerates `manifest.json`, fails on mismatch.
- [ ] `internal/platform`: OS/arch, glibc version, library probes
      (`vulkan-1.dll`, `nvcuda.dll`, `libvulkan.so.1`, `libcuda.so.1`),
      recommendation table from the installer spec, with fakeable probes.

Review forecast: about 380 lines.

## T3. Acquire

- [ ] `internal/acquire`: HTTP downloader with range resume, progress callback,
      SHA-256 verification, atomic placement, zip member extraction for cuBLAS.
- [ ] Tests with `httptest` servers simulating interruption and corruption.

Review forecast: about 350 lines.

## T4. Config and home layout

- [ ] `internal/config`: defaults, TOML load and save, env overrides,
      validation, `state/` helpers.
- [ ] Golden test for the default `config.toml`.

Review forecast: about 250 lines.

## T5. JEV types and providers

- [ ] `internal/jev`: request and answer types, `Provider` interface, `local`
      provider with `/predict` batching and 503 backoff, `typesafe` provider
      with bearer auth and 429/529 backoff.
- [ ] Golden tests using recorded engine responses from T0.

Review forecast: about 380 lines.

## T6. Sidecar supervision

- [ ] `internal/sidecar`: lock, state file, detached spawn per OS, readiness
      wait, health cache, client registration, idle reaper, log rotation,
      overload retry.
- [ ] Fake engine binary for tests; tests for lock races, stale lock, crash
      restart limit, idle stop.

Review forecast: about 400 lines; may split spawn (T6a) and lifecycle (T6b).

## T7. Tools: preflight, decide, check

- [ ] `internal/tools`: preflight estimator, confidence contract, `decide`,
      `check` with the two-option workaround and key randomisation.
- [ ] Golden tests with the fake provider.

Review forecast: about 350 lines.

## T8. Tools: classify, screen, rerank

- [ ] Batching against `max_questions`, order preservation, `top_k`, partial
      results with a deadline.
- [ ] Golden tests.

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

## Deferred (v2 candidates)

- Apple Core ML with published compiled buckets.
- Calibration store and temperature fitting from user-labelled outcomes.
- Multi-process routing across checkpoints by language.
- Optional gentle-ai subagent and prompt hooks (screen in research, classify
  in triage, check in verify) once the tools have measured precision.
- Homebrew tap, Scoop bucket, winget manifest.
- minisign release signatures.
