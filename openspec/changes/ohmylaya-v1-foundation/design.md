# Design: ohmylaya v1 foundation

## Overview

```
 agent host (Claude Code / Codex / OpenCode / Pi)
   │ stdio (MCP)
   ▼
 ohmylaya mcp ──────────► internal/tools ──► internal/jev.Provider
   │                                              │ local            │ typesafe
   │ ensure()                                     ▼                  ▼
   ▼                                        http://127.0.0.1:45292   https://api.typesafe.ai
 internal/sidecar (lock, spawn, health, idle)     │
   │                                              ▼
   └── $OHMYLAYA_HOME/bin/laya-cli --server ... ──┘
                       models/laya/<variant>/
```

One binary, four roles selected by subcommand. Nothing runs as a system
service. The sidecar is a plain child process that outlives the MCP server
that spawned it, so the next agent session finds it warm.

## Decisions

### D1. Go, single static binary

Rationale: one-shot install without a runtime; Bubble Tea for the TUI; official
MCP Go SDK; trivial child-process supervision; same toolchain as the ecosystem
this is meant to sit beside. `CGO_ENABLED=0` everywhere. GPU detection uses
file probes and `syscall.LoadLibrary` on Windows rather than cgo.

Alternative rejected: Node. `npx` is a good one-liner but requires Node, and
Windows PATH and Store-alias problems already sank the Python-based Laya MCPs.

### D2. Engine is upstream laya.cpp, never compiled here

ohmylaya downloads the release executables from `lkarlslund/laya.cpp`. The
release matrix (r0002):

| Asset | Size | Notes |
|---|---:|---|
| `laya-rNNNN-windows-amd64-vulkan.exe` | 80 MB | needs `vulkan-1.dll` from the GPU driver |
| `laya-rNNNN-windows-amd64-cuda.exe` | 200 MB | needs `cublas64_13.dll`, `cublasLt64_13.dll` beside it |
| `laya-rNNNN-linux-amd64-vulkan` | 82 MB | needs `libvulkan.so.1`, glibc 2.39+ |
| `laya-rNNNN-linux-amd64-cuda` | 786 MB | cuBLAS statically linked |
| `laya-rNNNN-macos-arm64-coreml` | 38 MB | Core ML needs exported buckets; v1 uses `--cpu` |

Windows cuBLAS DLLs come from NVIDIA's redistributable archive
`libcublas-windows-x86_64-13.1.0.3-archive.zip` (digest pinned in the
manifest); ohmylaya extracts only the two DLLs and the license.

Digests come from the release `SHA256SUMS` and are copied into the manifest
at ohmylaya release time by `go run ./internal/manifestgen`, which fails when
the upstream checksums file disagrees with the sha256 digest GitHub computes
for each release asset. Cross-checking two independent upstream sources avoids
downloading about 1.2 GB of executables per run; the installer still verifies
every download against the manifest. Model files take the Hugging Face LFS
oid; small non-LFS files are downloaded and checked against their git blob oid.
`-check` fails when the embedded manifest has drifted from upstream, and
branch revisions are pinned to their commit.

### D3. Backend defaults

- Windows and Linux: `vulkan` by default. Works on NVIDIA, AMD and Intel with
  the driver alone, 80 MB, and upstream validated FP16/BF16 on both vendors.
  `cuda` is opt-in: faster on NVIDIA but 200 MB plus DLLs on Windows and
  786 MB on Linux.
- macOS: `cpu` in v1. Core ML requires a PyTorch export plus Xcode compile of
  about 1.6 GiB per batch bucket; no one publishes compiled buckets. v2 can add
  Core ML once ohmylaya or upstream publishes them.
- `cpu` is the universal fallback. laya.cpp exposes `--cpu` in every build.
  T0 verified on Windows that the Vulkan executable serves on `--cpu` (ready in
  4 s, about 0.5 s per three-question request on a Ryzen 7) and on `--vulkan`
  (about 45 ms on an RTX 3050 Laptop) with identical answers. Still open: a
  Windows or Linux host with no Vulkan loader at all, and macOS `--cpu`. See
  `docs/research/engine-verification.md`.

### D4. Default model: `multilingual`

647 MB, 1024-token context, 100+ languages, and upstream measures it about 2x
faster than the English checkpoint. English-only users can pick `english`;
`typed-decisions` is offered for the four workflows it was fine-tuned on.
Checkpoint files per variant on Hugging Face `convaiinnovations/laya`:
`model.safetensors`, `rl_agent_config.json`, `encoder/*`, `tokenizer/*`; the
English variant lives at the repository root, others under `<variant>/`.
Files are fetched with `https://huggingface.co/<repo>/resolve/<revision>/<path>`
and verified with the LFS SHA-256 the API reports. No `huggingface_hub`.

### D5. Port 45292, loopback, discovered from state

"LAYA" on a phone keypad is 5292; 45292 is unassigned and far from the
8000/8080/8765/8787 range other local AI tools use. Fallback scan
45292..45299. Clients never assume the port; they read `state/sidecar.json`
(`{pid, port, backend, model, started_at, engine_digest}`) and confirm via
`/health`.

### D6. Sidecar ownership

`internal/sidecar.Ensure(ctx)` is the only way to obtain an engine. It:

1. Reads `state/sidecar.json`; if the PID is alive and `/health` is ready and
   the digest and model match config, attaches.
2. Otherwise takes `state/sidecar.lock` (`O_EXCL` create with PID; on Windows
   the same via `CreateFile` semantics through `os.OpenFile`), re-checks, spawns
   `laya-cli` detached (`setsid` on POSIX, `CREATE_NEW_PROCESS_GROUP` and
   `DETACHED_PROCESS` on Windows), waits for ready, writes state, releases lock.
3. Registers as an attached client by touching `state/clients/<pid>`; the idle
   reaper (a goroutine inside whichever ohmylaya process spawned the sidecar,
   plus a lazy check by any later client) stops the engine when no live client
   files exist and the last request is older than `idle_timeout`.

Because the spawning MCP process may die first, every attaching client runs the
reaper check on its own health ticks. Stopping is idempotent.

### D7. JEV as the wire contract

`internal/jev` defines `Request{State any; Questions map[string]Question; Model string}`
and `Answer` types matching TypeSafe's public schema and laya.cpp's
`/v1/systemone`. `local` posts to `/predict` with an array when a tool needs
more than one request, honouring `max_questions` per call by splitting. The
`typesafe` provider posts single requests to the hosted endpoint with a bearer
token and maps 429/529 to backoff. Tools never know which provider answered.

### D8. Tool layer contract

Each tool is a pure function `func(ctx, Provider, Input) (Output, error)` in
`internal/tools`, tested with a fake provider and golden JSON. The MCP layer in
`internal/mcpserver` only does schema and marshalling, so tools are reusable
from `ohmylaya ask` (a CLI subcommand that calls any tool from JSON on stdin,
useful for Pi scripts and shell pipelines).

Preflight: characters per token is estimated at 3.2 for `multilingual`
(SentencePiece with 256k vocabulary) and 3.8 for the byte-level BPE
checkpoints, then compared with the state budget (`max_len - head_max_len`).
Conservative on purpose; the goal is to warn, not to be exact.

`check` uses the two-option `choice` workaround because upstream issue #156
documents `noul` following its `false:`/`true:` labels. Keys `A` and `B` are
randomised per claim so position bias cannot lock to one key.

### D9. Agent adapters

`internal/agents` holds one package per agent behind an `Adapter` interface.
Writes go through `internal/cfgfile`, which offers `MergeJSON`, `MergeJSONC`
(comment-preserving via a tolerant parser that keeps the original text and
splices the new key), and `MergeTOML` (append or replace a single table using
line-level edits so comments survive). Every write backs up first and is
atomic. Golden tests hold real-world samples per agent and schema version.

| Agent | MCP registry | Skill directory |
|---|---|---|
| Claude Code | `~/.claude.json` `mcpServers.ohmylaya` (`type: stdio`) | `~/.claude/skills/ohmylaya/` |
| Codex | `~/.codex/config.toml` `[mcp_servers.ohmylaya]` | `~/.agents/skills/ohmylaya/` |
| OpenCode | `$XDG_CONFIG_HOME/opencode/opencode.json[c]` `mcp.ohmylaya` (v1) or `mcp.servers.ohmylaya` (v2), `type: local`, `command: [path, "mcp"]` | `~/.config/opencode/skills/ohmylaya/` |
| Pi | `~/.pi/agent/mcp.json` `mcpServers.ohmylaya` with `lifecycle: lazy` (file owned by `pi-mcp-adapter`) | `~/.agents/skills/ohmylaya/` |

`~/.agents/skills` is written once and shared by Codex and Pi. Nothing under
`~/.pi/agent` other than `mcp.json` is ever touched. No file belonging to
gentle-ai or any other framework is read or written.

### D10. TUI

Bubble Tea v1 with a root model that owns a `screen` enum and a shared
`services` struct (installer, sidecar, doctor, updater) so screens stay thin.
`huh` forms for Setup, `bubbles/progress` for downloads, `bubbles/viewport`
for logs. A single `theme.go` with lipgloss adaptive colours. Every screen
action maps to a subcommand so the TUI is a front-end, not a second code path.

### D11. Update

Self-update follows the GoReleaser layout: `checksums.txt` plus per-platform
archives. Windows swaps via `ohmylaya.exe.new` and a rename on next start.
After a self-update the new binary runs `install --yes --reuse` which
re-applies registrations with the possibly new absolute path and reconciles
engine and model against the new manifest. The `signature` field is reserved
for minisign.

### D12. Configuration

`config.toml`:

```toml
version = 1
backend = "vulkan"          # vulkan | cuda | cpu
model = "multilingual"      # multilingual | english | typed-decisions
port = 45292
idle_timeout = "30m"
provider = "local"          # local | typesafe

[precision]
strict = false              # true disables tensor-core and flash paths

[batch]
max_questions = 8
max_pending = 32
wait_ms = 2

[tools]
auto_accept = 0.8

[agents]
registered = ["claude", "codex", "opencode", "pi"]
```

Environment overrides: `OHMYLAYA_HOME`, `OHMYLAYA_PORT`, `OHMYLAYA_PROVIDER`,
`TYPESAFE_API_KEY`, `OHMYLAYA_LOG_LEVEL`.

### D13. Repository layout

```
cmd/ohmylaya/main.go
internal/
  acquire/      downloader, resume, digest, atomic place, HF and GitHub sources
  agents/       adapter.go, claude/, codex/, opencode/, pi/
  cfgfile/      JSON, JSONC, TOML merge with backup and atomic write
  config/       config.toml load, defaults, env overrides
  doctor/       checks and report
  jev/          types, local provider, typesafe provider
  manifest/     embedded manifest.json and types
  manifestgen/  release-time generator (not shipped)
  mcpserver/    MCP wiring
  platform/     os, arch, gpu, driver probes
  sidecar/      lock, spawn, health, idle, logs
  skill/        embedded skill files and installer
  tools/        decide, classify, check, screen, rerank, preflight
  tui/          app, screens, theme
  update/       self-update, engine and model reconcile
skills/ohmylaya/          canonical skill source (embedded via go:embed)
install.sh, install.ps1
docs/                     user docs, honest-limits, research
openspec/
```

### D14. Testing strategy

- Unit tests everywhere with table-driven cases; golden files under
  `testdata/`.
- `internal/jev` and `internal/tools` use recorded laya.cpp responses from the
  upstream smoke corpus so tests never need the engine.
- `internal/agents` golden tests start from real config samples.
- `internal/sidecar` tests use a fake engine binary (a tiny Go program built
  in `TestMain`) that serves `/health` and `/predict`.
- `//go:build engine` integration tests run the real engine when
  `OHMYLAYA_TEST_ENGINE=1`; CI runs them on a self-hosted GPU runner when one is
  available and skips otherwise.
- CI matrix: ubuntu, windows, macos; `go vet`, `staticcheck`, `go test -race`.

### D15. Token economics: references, not payloads

An MCP tool call is not free for the agent. Whatever the agent puts in the
arguments is emitted as output tokens, the most expensive kind. A `rerank`
that makes the agent paste sixty file bodies into the call would cost more
than reading them. So tools accept references (`path`, `url`, `glob`) and
ohmylaya reads the content itself. Savings come from three places, in order of
size:

1. Content that never enters context: `screen` on a URL that ends in `block`
   or `skip` saves the whole page and the reasoning about it.
2. Reading fewer things: `rerank` over a glob returns five ids instead of the
   agent reading sixty files to find the right one.
3. Decisions that leave the model entirely: `ohmylaya ask` from scripts, CI
   and hooks costs zero model tokens.

Where Laya replaces a judgment the model would have made inline on content
already in context, the saving is small and the gain is the calibrated
probability, not the tokens. Task T18 measures all of this on real sessions
before any number appears in the README.

## Open questions

1. Go module path: `github.com/QuBiit0/ohmylaya`, personal account, decided 2026-09-23.
2. Whether the one-shot scripts live at a vanity domain or stay on raw GitHub
   URLs. Design assumes raw GitHub for v1.
3. T0 outcomes for CPU fallback on Vulkan and macOS executables.
