# Research: landscape and evidence (September 2026)

Collected before writing the v1 specs. Links were live on 2026-09-23.

## The model: Laya

- Repository: `convaiinnovations/laya` on Hugging Face, Apache-2.0, by Convai
  Innovations (Nandakishor M). Non-autoregressive "System 1" decision model.
  Input: a state plus typed questions (`choice`, `score`, `noul`). Output:
  typed answers with probabilities and a confidence. Never generates text.
- Three checkpoints in one repository:

| Variant | Encoder | Params | Context | Weights | Path |
|---|---|---:|---:|---:|---|
| `english` | ModernBERT-large | 421M | 512 | 843 MB | repository root |
| `multilingual` | mmBERT-base | 322M | 1024 | 644 MB | `multilingual/` |
| `typed-decisions` | ModernBERT-large | 421M | 1024 | 843 MB | `typed-decisions/` |

  Each variant also needs `rl_agent_config.json`, `encoder/*` and
  `tokenizer/*` (tokenizer.json about 3.6 MB for English).
- Upstream latency on a T4: 33 to 40 ms for one question, about 7 ms per
  question at batch 10 on the multilingual checkpoint.
- Honest limits documented upstream: base checkpoints near chance on
  zero-shot typed decisions (0.362 vs 0.461 majority baseline); `score` is
  the weakest primitive; more than 20 options degrade sharply because options
  share a 192 or 256 token budget; `noul` can follow its labels instead of the
  state (issue #156), workaround is a two-option `choice`;
  `action.act_probability` carries no signal (issue #185); calibration error
  0.466 before temperature fitting; state is truncated from the end silently.
- Python package `laya` 0.3.7 ships `laya-serve`, a JEV-compatible HTTP
  server, and a `Router` that picks a checkpoint by script and language.

## The hosted alternative: TypeSafe Jev

- Closed weights, waitlist, `POST https://api.typesafe.ai/v1/systemone`,
  bearer token, $0.042 per million input tokens, about 250 ms p50 measured by
  third parties. Same request and answer schema that laya.cpp implements.
- Jev leads Laya on high-cardinality choices (Banking77: 0.870 vs 0.425) and
  soft distribution matching; Laya leads on argmax accuracy for the
  typed-decisions benchmark, speed, calibration after fitting, cost and
  languages.

## The engine: laya.cpp

- `lkarlslund/laya.cpp`, MIT. C++20 over ggml with CUDA, Vulkan and Core ML
  backends plus a `--cpu` backend. Tokenizer, inference and JSON in C++.
- HTTP server: `POST /v1/systemone` (JEV schema, one request),
  `POST /predict` (array, CLI envelope with `results`, `elapsed_ms`,
  `backend`), `GET /health`, `GET /v1/models`. Optional `LAYA_API_KEY`.
- Concurrency: one inference worker per process. Bounded FIFO queue,
  defaults `--max-questions 8`, `--max-pending-requests 32`,
  `--batch-wait-ms 2`. Overflow returns 503 with `Retry-After: 1`. Socket
  timeouts 10 s. SIGINT and SIGTERM drain then exit.
- One checkpoint per process; no language routing.
- Rolling prereleases `rNNNN` with five raw executables, `SHA256SUMS`,
  `RUNTIME-REQUIREMENTS.md`, NOTICES. r0002 sizes: linux cuda 786 MB, linux
  vulkan 82 MB, windows cuda 200 MB, windows vulkan 80 MB, macos coreml 38 MB.
- Windows CUDA needs `cublas64_13.dll` and `cublasLt64_13.dll` from NVIDIA's
  `libcublas-windows-x86_64-13.1.0.3-archive.zip`
  (sha256 `4ac4847bbe4f7709b244956fcfc32197a2954ee70b155cb67eebd9ee26f7e339`).
- Linux needs glibc 2.39+; Vulkan builds need `libvulkan.so.1`.
- Core ML needs a PyTorch export and `xcrun coremlcompiler`; about 1.6 GiB per
  batch bucket; not viable for a one-shot install today.
- Upstream numbers on an RTX PRO 6000: 250 to 700 questions per second
  depending on checkpoint and batch; CUDA 1.3x to 2.7x over matching Python.
- GPU acceptance is not run by the hosted CI, so releases are prereleases.

## Existing integrations

| Project | Stack | Notes |
|---|---|---|
| `jkudish/jev-mcp` | Node, MIT | Reference Jev MCP, ten tools (verify, screen, find, rerank, classify, decide, compare, extract, review, gate). Supports `TYPESAFE_BASE_URL` and a generic `JEV_API_BASE_URL` provider |
| `itsmostafa/typesafe-mcp` | Node | Jev MCP |
| `andragon3110/laya-mcp` | Node MCP + Python FastAPI | Ten tools cloned from jev-mcp; installs a venv, torch and all three checkpoints |
| `jerepaira/laya-mcp` | Python stdio | Four tools (decide, classify, score, check); pulls torch |
| `PerryLink/laya-mcp` | npm launcher + Python | Adds preflight for truncation, honest confidence, `doctor`, `install` for Claude, Codex, opencode, OpenClaw, Hermes; ships a SKILL.md; reports Pi unsupported |

Every Laya MCP depends on Python and PyTorch. None uses laya.cpp. That is the
gap ohmylaya fills.

## Agent configuration surfaces

| Agent | MCP registry | Skill directory | Source |
|---|---|---|---|
| Claude Code | `~/.claude.json` `mcpServers` (`type: stdio`, `command`, `args`, `env`); project `.mcp.json`; `claude mcp add -s user` | `~/.claude/skills/<name>/SKILL.md` | code.claude.com docs |
| Codex | `~/.codex/config.toml` `[mcp_servers.<name>]` `command`, `args`, `env`, `enabled` | `~/.agents/skills/<name>/SKILL.md` (real directory, symlinks are not followed, issue #11314) | developers.openai.com |
| OpenCode | `opencode.json[c]` `mcp.<name>` `{type: local, command: [...]}` (v1) or `mcp.servers.<name>` (v2, `disabled` instead of `enabled`) | `~/.config/opencode/skills/` | opencode.ai docs |
| Pi | `~/.pi/agent/mcp.json` `mcpServers.<name>` `{command, args, directTools?, lifecycle?}` owned by `pi-mcp-adapter` | `~/.agents/skills/`, `~/.pi/agent/skills/` | local inspection of a gentle-pi install |

## gentle-ai and gentle-pi

Inspected the installed `gentle-pi` package and the gentle-ai source.

- gentle-ai is a Go project (Bubble Tea 1.3, lipgloss 1.1, GoReleaser,
  minisign) with per-agent adapters under `internal/agents/<agent>` and a
  `filemerge` component that merges overlays, backs up and retries. Skills are
  embedded assets with `name`, `description` starting with `Trigger:`,
  `license`, `metadata.author`, `metadata.version`. Body limit about 700 tokens.
- The skill registry (`.atl/skill-registry.md`) scans `~/.agents/skills`,
  `~/.claude/skills`, `~/.config/opencode/skills`, `~/.copilot/skills` and
  project `.agents/skills`. It skips `sdd-*`.
- Subagents (`sdd-*`, `review-*`, `jd-*`) are Markdown files with frontmatter
  under `~/.pi/agent/agents/`; they list `mcp` as a tool, so any registered
  MCP server is reachable from them without changes.
- Conclusion: gentle-ai is declarative and registry-driven. A third-party tool
  integrates with one MCP entry and one SKILL.md. No hook or framework file
  needs to be touched, and ohmylaya must never touch one.

## Natural future touch points (deferred, opt-in, gentle-ai side)

- `sdd-research` and `sdd-explore`: `screen` fetched content before it enters
  context.
- `systemic-issue-triage`: `classify` and `rerank` issues by root class.
- `sdd-verify` and RDD: `check` claims against captured evidence before the
  expensive verifier runs.
- Judgment Day: `rerank` findings and detect agreement between judges.
- Orchestration: `decide` routing questions with 3 to 6 options.

These belong to gentle-ai's roadmap, not ohmylaya's, and should wait for
measured precision on real repositories.
