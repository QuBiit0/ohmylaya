# Changelog

All notable changes to ohmylaya. Versions follow [Semantic Versioning](https://semver.org/);
while the version is 0.x, minor releases may change defaults.

## [0.1.2] - 2026-10-04

### Changed

- **`english` is the default checkpoint.** It won every tool on the
  benchmark; `multilingual` was near or below chance on `rerank` and
  `screen`. Existing installs keep the model in their `config.toml`, and an
  interactive rerun preselects that configured model.
- `screen` and `check` are documented as hints, with their measured
  accuracy, in the README, the skill and the MCP tool descriptions. The skill
  no longer tells agents to read a page only when `screen` says `allow`.

### Fixed

- The installer refuses hosts the engine cannot start on, before
  downloading anything, and names the fix:
  - Windows or Linux with neither the Vulkan loader nor the NVIDIA driver.
    The CPU backend runs the Vulkan build, which needs the loader even
    without a GPU, so `cpu` is offered only where the loader exists.
  - macOS older than 15. The upstream engine targets macOS 15.
- CUDA on Windows installs again. The cuBLAS DLLs were looked up in the
  wrong directory of NVIDIA's archive.
- `ohmylaya doctor` no longer suggests `--backend cpu` for a missing Vulkan
  loader.
- The engine start retries on another port when a different process takes
  the chosen one first.

### Added

- `install-smoke`: CI installs every release with the one-shot scripts on
  clean Linux, macOS 15 and Windows runners and makes a real tool call.
- CUDA latency on Windows in `docs/benchmarks.md`.

## [0.1.1] - 2026-09-24

### Fixed

- Engines started by short-lived commands (`ohmylaya ask`, the install smoke
  test) now stop when idle instead of staying on the GPU. Each reaper is bound
  to its engine, and `ohmylaya mcp` keeps a fallback reaper.
- Uninstall retries removal while Windows still holds files.
- Engine calls are packed by state size as well as question count.

## [0.1.0] - 2026-09-23

First release.

- One-shot installers for Linux, macOS and Windows that verify checksums.
- `ohmylaya install`: backend detection, verified downloads of the laya.cpp
  engine and a Laya checkpoint, smoke test, and agent registration with
  backups.
- MCP tools `decide`, `classify`, `check`, `screen` and `rerank`, also
  callable with `ohmylaya ask`.
- A shared engine that starts on demand and stops when idle.
- Registration for Claude Code, Codex, OpenCode and Pi, plus an agent skill.
- `ohmylaya doctor`, `ohmylaya update`, `ohmylaya uninstall` and a TUI.

[0.1.2]: https://github.com/QuBiit0/ohmylaya/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/QuBiit0/ohmylaya/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/QuBiit0/ohmylaya/releases/tag/v0.1.0
