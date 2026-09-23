# Proposal: ohmylaya v1 foundation

## Intent

Coding agents make thousands of small judgments per session: is this fetched
page safe to read, which of these files answers the question, does this claim
match the evidence, which label fits this issue. Today each judgment is either
skipped or paid to a frontier model at hundreds of milliseconds and real cost.

Laya is an open-weight decision model that answers typed questions with
calibrated probabilities in about 30 ms on a GPU. laya.cpp runs it natively
without Python. Nobody ships the two together in a form an agent can use in one
command. Existing Laya MCP servers all depend on Python and a 2 GB PyTorch
install. ohmylaya closes that gap.

## Scope

### In scope

- One static Go binary, `ohmylaya`, for Windows x64, Linux x64, Linux arm64,
  macOS arm64.
- `ohmylaya install`: download the pinned laya.cpp executable and one Laya
  checkpoint, verify checksums, detect a usable backend, register the MCP server
  and skill in the selected agents. Works non-interactively with flags.
- One-shot install scripts (`install.sh`, `install.ps1`) that fetch the binary
  and run `ohmylaya install`.
- Sidecar supervision: start laya.cpp on demand on a loopback port, share one
  instance across agent sessions, stop it after an idle timeout, expose health.
- MCP stdio server with five tools: `decide`, `classify`, `check`, `screen`,
  `rerank`. Each returns typed answers, probabilities, confidence and a
  truncation preflight.
- Agent skill `ohmylaya` (SKILL.md) installed into the skill directories of the
  selected agents.
- Registration adapters for Claude Code, Codex, OpenCode and Pi.
- `ohmylaya doctor`: diagnose OS, GPU, drivers, binaries, models, port, sidecar,
  agent registrations. Human and JSON output.
- `ohmylaya update`: self-update the binary, bump the engine and checkpoint to
  the versions pinned by the new binary.
- Bubble Tea TUI (`ohmylaya` with no arguments): status, setup wizard, agents,
  update, doctor, logs.
- Hosted-provider mode: point the same tools at TypeSafe Jev with an API key,
  because laya.cpp already speaks the JEV schema.

### Out of scope for v1

- Fine-tuning, temperature calibration store, dataset export.
- Apple Core ML backend (needs multi-GiB exported buckets that nobody publishes
  yet). macOS v1 runs the CPU backend.
- Remote or multi-user serving, TLS, authentication beyond loopback.
- Automatic language routing between checkpoints (laya.cpp loads one
  checkpoint per process; v1 runs one process).
- Optional gentle-ai subagents or prompt hooks. gentle-ai integration in v1 is
  discovery through MCP registration and the skill only.
- Package-manager distribution (Homebrew, Scoop, winget) beyond a documented
  plan.

## Approach

ohmylaya never implements inference. It owns four things: acquisition
(download, verify, pin), supervision (spawn, health, idle stop), translation
(MCP tools to JEV requests and back, with preflight and honest confidence), and
integration (skill, agent config merge, doctor, TUI).

The engine is always the upstream laya.cpp release executable, pinned in an
embedded manifest with SHA-256 digests. The checkpoint is always the upstream
Hugging Face repository at a pinned revision. Upgrading either means shipping a
new ohmylaya release with a new manifest, so the whole stack is reproducible
from the ohmylaya version alone.

## Affected areas

| Area | Impact | Description |
|---|---|---|
| `cmd/ohmylaya` | New | Entry point, subcommand routing |
| `internal/manifest` | New | Embedded engine and checkpoint pins |
| `internal/acquire` | New | Downloads, checksums, resume, atomic placement |
| `internal/platform` | New | OS/arch/GPU/driver detection, backend selection |
| `internal/sidecar` | New | laya.cpp process supervision, lock, health, idle stop |
| `internal/jev` | New | JEV request and answer types, local and hosted clients |
| `internal/tools` | New | The five MCP tools, preflight, confidence contract |
| `internal/mcpserver` | New | MCP stdio server wiring |
| `internal/agents` | New | Adapters for Claude Code, Codex, OpenCode, Pi |
| `internal/skill` | New | Embedded SKILL.md and installer |
| `internal/doctor` | New | Checks and report |
| `internal/update` | New | Self-update and engine bump |
| `internal/tui` | New | Bubble Tea screens |
| `install.sh`, `install.ps1` | New | One-shot bootstrap |
| `docs/` | New | User docs, research notes, honest limits |

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| laya.cpp releases are prereleases without GPU validation | High | Pin a tested tag; run our own smoke corpus in CI on a GPU runner when available; expose `doctor --smoke` |
| Vulkan executable fails to start without a Vulkan loader, blocking the CPU fallback | Medium | Verify in task T0; if confirmed, request an upstream CPU-only or dual build, or ship the CUDA/Vulkan choice plus a documented CPU path |
| macOS Core ML binary may not honour `--cpu` | Medium | Verify in task T0 on Apple Silicon; fall back to asking upstream for a CPU build |
| Base checkpoints are near chance on zero-shot typed decisions | Certain | Skill states it plainly; tools default to the workloads Laya is good at; README leads with honest limits |
| `noul` follows its labels instead of the state | Known | `check` tool asks a two-option `choice` with neutral keys, per upstream guidance |
| Silent state truncation | Known | Every tool runs a preflight and returns `truncated` with the estimated cut |
| Windows cannot send SIGTERM to the sidecar | Certain | Idle stop uses `Process.Kill` after draining; document that an in-flight batch may be lost |
| Agent config formats change (OpenCode v2 moved servers under `mcp.servers`) | Medium | Adapters detect the schema version and are covered by golden tests |
| Upstream renames or removes release assets | Medium | Manifest pins full URLs and digests; update fails closed |
| Large downloads on slow links | High | Resume support, progress, checksum after download, never partial placement |

## Rollback plan

`ohmylaya uninstall` removes `$OHMYLAYA_HOME` and the `ohmylaya` entries from
each agent config using the backups it made. No other files are touched.
Agents keep working without ohmylaya because tools are only advertised when the
sidecar is healthy.

## Dependencies

- laya.cpp release `r0002` or newer with the five-asset matrix and `SHA256SUMS`.
- Hugging Face repository `convaiinnovations/laya` at the pinned revision.
- Go 1.26 toolchain for contributors only.

## Success criteria

- [ ] A fresh Windows, Linux or macOS machine reaches a working `decide` call in
      one command plus one agent restart, with no Python, CUDA toolkit or Node.
- [ ] `ohmylaya doctor` explains every failure in one sentence with a fix.
- [ ] The five tools pass golden tests against recorded laya.cpp responses.
- [ ] Registration is idempotent and never destroys existing agent config.
- [ ] `go test -race ./...` passes on all three platforms in CI.
- [ ] README states what Laya is bad at before it states what it is good at.
