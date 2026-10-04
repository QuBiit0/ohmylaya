# ohmylaya

[![CI](https://github.com/QuBiit0/ohmylaya/actions/workflows/ci.yml/badge.svg)](https://github.com/QuBiit0/ohmylaya/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/QuBiit0/ohmylaya)](https://github.com/QuBiit0/ohmylaya/releases/latest)
[![Release workflow](https://github.com/QuBiit0/ohmylaya/actions/workflows/release.yml/badge.svg)](https://github.com/QuBiit0/ohmylaya/actions/workflows/release.yml)
[![Go](https://img.shields.io/github/go-mod/go-version/QuBiit0/ohmylaya)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**A local, calibrated decision engine for your coding agents, installed in one
command.**

ohmylaya downloads and supervises [laya.cpp](https://github.com/lkarlslund/laya.cpp),
a native runtime for the open-weight [Laya](https://huggingface.co/convaiinnovations/laya)
typed-decision models. It exposes them to Claude Code, Codex, OpenCode and Pi
as an MCP server plus an agent skill, so your agent can rank files, label
items and sanity-check claims locally, with a probability attached, instead
of spending frontier-model tokens. No Python, no PyTorch, no Node.

## Quick start

Linux and macOS:

```sh
curl -fsSL https://raw.githubusercontent.com/QuBiit0/ohmylaya/main/install.sh | sh
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/QuBiit0/ohmylaya/main/install.ps1 | iex
```

Then restart your agent. The installer:

1. verifies the `ohmylaya` binary against the release checksums;
2. checks that the engine can run on your machine, before downloading anything;
3. downloads the engine and the `english` checkpoint and verifies their digests;
4. runs a smoke test;
5. registers ohmylaya with every agent it detects, backing up each config file first.

Check it any time with `ohmylaya doctor`.

## Read this first: what it is not

- **It does not review your code.** Laya's ordinal `score` primitive is its
  weakest, and long inputs are truncated from the end. Use your frontier
  model for review.
- **It is not a zero-shot oracle.** The base checkpoints score near chance
  on domain-specific typed decisions without fine-tuning.
- **It collapses past about 20 options per question.**
- **Its probabilities ship over-confident** until calibrated on your data.
- **`screen` and `check` are weak.** Treat their answers as hints to inspect,
  never as verdicts. The numbers are below.

## What it is good at

Fast, cheap, typed judgments with a probability attached. Inside the engine a
question takes about 15 ms on a laptop GPU. A whole tool call from the CLI
takes 140 to 640 ms (a few seconds for `rerank` over 20 files), and hundreds
of milliseconds more on a CPU.

| Tool | Use it to | Measured accuracy* |
|---|---|---:|
| `rerank` | order files or candidates by relevance to a query, with no embeddings or index | 80–90% |
| `classify` | label items against your own catalogue, in batches | 75% |
| `check` | get a hint on whether claims match evidence, such as "tests passed" against a log | 57–71% |
| `screen` | get a hint on whether a page or file looks like prompt injection, thin or off-topic | 45% |
| `decide` | ask any typed question when you need the raw primitive | n/a |

\* `english` checkpoint on a small hand-labelled corpus. The method, token
savings and failure cases are in [docs/benchmarks.md](docs/benchmarks.md).

Tools accept `path`, `url` and `glob` references and read the content
themselves, so it never passes through the agent's context. Every answer
carries probabilities and an `auto` or `review` action, so agents can branch
and escalate instead of guessing.

## How it works

```
 Claude Code ─┐                          ┌─────────────────────────────┐
 Codex ───────┤   MCP over stdio         │ laya-cli (laya.cpp)         │
 OpenCode ────┼──► ohmylaya mcp ──HTTP──►│ one shared local engine     │
 Pi ──────────┘    ohmylaya ask (CLI)    │ 127.0.0.1 · Vulkan/CUDA/CPU │
                                         └─────────────────────────────┘
```

The engine starts on the first tool call and is shared by every agent
session. It stops after 30 minutes idle. Everything lives under
`~/.ohmylaya`.

## Use

```sh
ohmylaya              # TUI: status, setup, agents, update, doctor, logs
ohmylaya doctor       # one line per check, with a fix for each failure
ohmylaya agents       # detection and registration per agent
ohmylaya update       # newer binary, then engine, model and skill
echo '{"claims":["tests passed"],"ref":{"path":"test.log"}}' | ohmylaya ask check
```

Configuration, every command and flag, and how to call the engine from your
own scripts are in [docs/usage.md](docs/usage.md).

## Agents

| Agent | MCP registry | Skill |
|---|---|---|
| Claude Code | `~/.claude.json` | `~/.claude/skills/ohmylaya` |
| Codex | `~/.codex/config.toml` | `~/.agents/skills/ohmylaya` |
| OpenCode | `opencode.json` or `.jsonc` (v1 and v2 schemas) | `~/.config/opencode/skills/ohmylaya` |
| Pi | `~/.pi/agent/mcp.json` (needs `pi-mcp-adapter`) | `~/.agents/skills/ohmylaya` |

ohmylaya depends on no agent framework. It writes only the registry and skill
directory each agent already reads. It backs each file up first and restores
it byte for byte on uninstall.

## Requirements

| Platform | Backends | Notes |
|---|---|---|
| Windows 10 or 11, x64 | Vulkan, CUDA, CPU | The CPU backend runs the Vulkan build and needs the Vulkan loader: a GPU driver provides it, or install the [Vulkan Runtime](https://vulkan.lunarg.com/sdk/home). |
| Linux x64, glibc 2.39+ | Vulkan, CUDA, CPU | Ubuntu 24.04 or newer. The CPU backend needs `libvulkan1` (Debian, Ubuntu) or `vulkan-loader` (Fedora, Arch). |
| macOS 15+, Apple Silicon | CPU | Core ML is not available yet; upstream publishes no model buckets for it. |

CUDA needs the NVIDIA driver. The installer checks all of this and stops with
the exact fix before it downloads anything.

| Download | Size |
|---|---|
| Engine, Vulkan or CPU | about 80 MB (37 MB on macOS) |
| Engine, CUDA on Windows | 191 MB plus a 404 MB cuBLAS archive |
| Engine, CUDA on Linux | 786 MB |
| `english` checkpoint (default) | about 810 MB |
| `multilingual` checkpoint | about 650 MB |

Pick `--model multilingual` only for non-English content. It fell to near or
below chance on `rerank` and `screen` in the benchmark.

## Hosted mode

laya.cpp speaks the TypeSafe JEV schema, so the same tools can point at the
hosted Jev API. Set `provider = "typesafe"` in `~/.ohmylaya/config.toml` and
export `TYPESAFE_API_KEY`.

## Documentation

| Read | When you want to |
|---|---|
| [docs/usage.md](docs/usage.md) | configure ohmylaya, see every command, call it from scripts |
| [docs/limits.md](docs/limits.md) | know exactly where the model fails |
| [docs/benchmarks.md](docs/benchmarks.md) | see measured accuracy, latency and token savings |
| [docs/troubleshooting.md](docs/troubleshooting.md) | fix a failing install or tool call |
| [CHANGELOG.md](CHANGELOG.md) | see what changed between releases |
| [docs/research/](docs/research/) | read the evidence behind the design |

## Status

v0.1.2. Right after each release is published, CI installs it with the
one-shot scripts on clean Linux, macOS 15 and Windows machines and makes a
real tool call. Windows is also
verified end to end on real hardware with Vulkan, CUDA and CPU. Field
reports from other GPUs are welcome.

## Contributing

Issues and pull requests are welcome. Read [CONTRIBUTING.md](CONTRIBUTING.md)
and [AI_POLICY.md](AI_POLICY.md) first. Report security issues privately as
described in [SECURITY.md](SECURITY.md).

## License

MIT. Laya models are Apache-2.0 by Convai Innovations. laya.cpp is MIT by Lars
Karlslund. ohmylaya is an independent project and is not affiliated with
Convai Innovations, TypeSafe AI or the laya.cpp author.
