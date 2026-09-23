# Project Context: ohmylaya

## Purpose

ohmylaya is an open-source, single-binary tool that gives coding agents a fast,
local, calibrated decision engine in one command. It installs and supervises a
[laya.cpp](https://github.com/lkarlslund/laya.cpp) sidecar (native C++ inference
for the open-weight [Laya](https://huggingface.co/convaiinnovations/laya)
typed-decision models), exposes it as an MCP server with a small set of
typed-decision tools, ships an agent skill that teaches agents when and how to
use it, and registers itself in Claude Code, Codex, OpenCode and Pi.

Positioning: a local judge, not a code reviewer. Laya answers typed questions
(`choice`, `score`, `noul`) with calibrated probabilities in tens of
milliseconds. It never generates text. It is the cheap reflex that runs before
and after expensive frontier-model actions: screening fetched content, ranking
candidates, classifying at volume, checking claims against evidence.

## Stack

- Language: Go 1.26, `CGO_ENABLED=0`, one static binary per OS/arch.
- CLI: `cobra` or the standard library `flag` package (decided in design).
- TUI: Bubble Tea v1.x, bubbles, lipgloss, huh.
- MCP: `github.com/modelcontextprotocol/go-sdk` (stdio transport).
- Engine: laya.cpp release executables, pinned by tag and SHA-256 in an
  embedded manifest. Never compiled by ohmylaya.
- Models: Hugging Face `convaiinnovations/laya`, pinned by revision.
- Release: GoReleaser, GitHub Releases, checksums; minisign signing planned.

## Conventions

- All code, identifiers, comments, docs, commit messages and UI strings are in
  English.
- Work-unit commits: each commit is a reviewable unit that keeps tests and docs
  with the code they describe. Conventional commit prefixes.
- Pull requests stay under 400 changed lines; larger work is chained.
- Strict TDD once Go code exists: RED, GREEN, TRIANGULATE, REFACTOR. Table-driven
  tests and golden files. `go test ./...` must pass with `-race`.
- OpenSpec artifacts live under `openspec/`. Changes go through proposal, spec,
  design, tasks, apply, verify, archive.
- Never write outside `$OHMYLAYA_HOME` and the agent config files the user
  explicitly selected. Merge, never overwrite. Back up before every write.
- No telemetry. No network calls except the documented downloads and the
  optional hosted-provider mode the user turns on.

## Testing capability

Go standard testing (`go test ./...`). No code exists yet; TDD is mandatory from
the first Go file. Integration tests that need the real engine are tagged
`//go:build engine` and skipped by default.

## Naming

- Binary and command: `ohmylaya`.
- MCP server id in agent configs: `ohmylaya`.
- Tool names: `decide`, `classify`, `check`, `screen`, `rerank` (exposed by
  hosts as `mcp__ohmylaya__<tool>` or equivalent).
- Home directory: `$OHMYLAYA_HOME`, default `~/.ohmylaya`.
- Sidecar port: `45292` (LAYA on a phone keypad), loopback only.

## Trademarks and licensing

ohmylaya is MIT licensed. Laya models are Apache-2.0 by Convai Innovations.
laya.cpp is MIT by Lars Karlslund. ohmylaya is an independent integration and is
not affiliated with or endorsed by either project or by TypeSafe AI.
