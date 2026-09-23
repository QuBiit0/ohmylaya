# ohmylaya

A local, calibrated decision engine for your coding agents, installed in one
command.

ohmylaya downloads and supervises [laya.cpp](https://github.com/lkarlslund/laya.cpp),
a native runtime for the open-weight [Laya](https://huggingface.co/convaiinnovations/laya)
typed-decision models, and exposes it to Claude Code, Codex, OpenCode and Pi
as an MCP server plus an agent skill. No Python, no PyTorch, no Node.

## What it is not

Read this first.

- It does not review your code. Laya's ordinal `score` primitive is its
  weakest, and long inputs are truncated from the end. Use your frontier
  model for review.
- It is not a zero-shot oracle. The base checkpoints score near chance on
  domain-specific typed decisions without fine-tuning.
- It collapses past about 20 options per question.
- Its probabilities ship over-confident until calibrated on your data. The
  `screen` tool in particular reads imperative documentation as prompt
  injection about as often as real injections.

## What it is good at

Fast, cheap, typed judgments with a probability attached: about 15 ms per
question on a laptop GPU, hundreds of milliseconds on a CPU.

| Tool | Use it to |
|---|---|
| `screen` | judge a URL or file for prompt injection, substance and relevance before it enters context |
| `rerank` | order files or candidates by relevance to a query, no embeddings or index |
| `classify` | label items against your own catalogue, in batches |
| `check` | verify claims against evidence, such as "tests passed" against the log |
| `decide` | ask any typed question when you need the raw primitive |

Tools accept `path`, `url` and `glob` references and read the content
themselves, so it never passes through the agent's context as arguments.
Every answer returns probabilities and an `auto` or `review` action so agents
can branch, gate and escalate instead of guessing.

## Install

Linux and macOS:

```sh
curl -fsSL https://raw.githubusercontent.com/QuBiit0/ohmylaya/main/install.sh | sh
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/QuBiit0/ohmylaya/main/install.ps1 | iex
```

The script verifies the binary against the release checksums, then runs
`ohmylaya install`, which picks a backend for your machine, downloads the
engine and the multilingual checkpoint, runs a smoke test and registers the
detected agents. Restart your agent afterwards.

Downloads: engine 40 to 80 MB (Vulkan or CPU), 200 MB plus two cuBLAS DLLs
(CUDA on Windows), 786 MB (CUDA on Linux); checkpoint 650 to 850 MB.

Runtime needs: Windows 10 or 11 x64, Linux x64 with glibc 2.39+, or Apple
Silicon (CPU backend in this release). Vulkan needs a GPU driver; CUDA needs
the NVIDIA driver.

## Use

```sh
ohmylaya              # TUI: status, setup, agents, update, doctor, logs
ohmylaya doctor       # one line per check, with a fix for each failure
ohmylaya agents       # detection and registration per agent
ohmylaya update       # newer binary, then engine, model and skill
echo '{"claims":["tests passed"],"ref":{"path":"test.log"}}' | ohmylaya ask check
```

The engine starts on the first tool call, is shared across agent sessions,
and stops after 30 minutes idle. See [docs/usage.md](docs/usage.md).

## Agents

| Agent | MCP registry | Skill |
|---|---|---|
| Claude Code | `~/.claude.json` | `~/.claude/skills/ohmylaya` |
| Codex | `~/.codex/config.toml` | `~/.agents/skills/ohmylaya` |
| OpenCode | `opencode.json` or `.jsonc` (v1 and v2 schemas) | `~/.config/opencode/skills/ohmylaya` |
| Pi | `~/.pi/agent/mcp.json` (needs `pi-mcp-adapter`) | `~/.agents/skills/ohmylaya` |

ohmylaya depends on no agent framework. It writes only the registry and skill
directory each agent already reads, backs the file up first and restores it
byte for byte on uninstall, so tools such as gentle-ai discover it without
any coupling.

## Hosted mode

laya.cpp speaks the TypeSafe JEV schema, so the same tools can point at the
hosted Jev API: set `provider = "typesafe"` in `~/.ohmylaya/config.toml` and
export `TYPESAFE_API_KEY`.

## Status

v0.1.0. Verified end to end on Windows x64 with Vulkan and CPU. Linux and
macOS builds are produced by CI and need field reports; see
[docs/research/engine-verification.md](docs/research/engine-verification.md).
No token or accuracy claims are made until the benchmark in the roadmap
lands. Contributions welcome: read [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT. Laya models are Apache-2.0 by Convai Innovations. laya.cpp is MIT by Lars
Karlslund. ohmylaya is an independent project and is not affiliated with
Convai Innovations, TypeSafe AI or the laya.cpp author.
