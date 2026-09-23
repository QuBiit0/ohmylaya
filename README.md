# ohmylaya

A local, calibrated decision engine for your coding agents, installed in one
command.

ohmylaya downloads and supervises [laya.cpp](https://github.com/lkarlslund/laya.cpp),
a native runtime for the open-weight [Laya](https://huggingface.co/convaiinnovations/laya)
typed-decision models, and exposes it to Claude Code, Codex, OpenCode and Pi as
an MCP server plus an agent skill. No Python, no PyTorch, no Node.

> Status: specification phase. No code yet. See `openspec/changes/ohmylaya-v1-foundation`.

## What it is not

Read this first.

- It does not review your code. Laya's ordinal `score` primitive is its weakest,
  and long diffs are truncated from the end. Use your frontier model for review.
- It is not a zero-shot oracle. The base checkpoints score near chance on
  domain-specific typed decisions without fine-tuning.
- It collapses past about 20 options per question.
- Its probabilities ship over-confident until calibrated on your data.

## What it is good at

Fast, cheap, typed judgments with a probability attached, in tens of
milliseconds on a GPU and hundreds on a CPU:

- `screen` fetched pages for prompt injection, substance and relevance before
  they enter context.
- `rerank` candidates by relevance without embeddings or an index.
- `classify` items against your own label set, in batches.
- `check` claims against evidence, such as "tests passed" against the log.
- `decide` any typed question when you need the raw primitive.

Every answer returns probabilities, a confidence and an `auto` or `review`
action so agents can branch, gate and escalate instead of guessing.

## Install (planned)

```sh
curl -fsSL https://raw.githubusercontent.com/QuBiit0/ohmylaya/main/install.sh | sh
```

```powershell
irm https://raw.githubusercontent.com/QuBiit0/ohmylaya/main/install.ps1 | iex
```

Then restart your agent. `ohmylaya doctor` explains anything that is off.

## Agents

| Agent | MCP | Skill |
|---|---|---|
| Claude Code | `~/.claude.json` | `~/.claude/skills/ohmylaya` |
| Codex | `~/.codex/config.toml` | `~/.agents/skills/ohmylaya` |
| OpenCode | `opencode.json` | `~/.config/opencode/skills/ohmylaya` |
| Pi | `~/.pi/agent/mcp.json` (via `pi-mcp-adapter`) | `~/.agents/skills/ohmylaya` |

ohmylaya depends on no agent framework. It only writes the MCP registry and
skill directory each agent already reads, so tools such as gentle-ai discover
it without any coupling.

## Hosted mode

laya.cpp speaks the TypeSafe JEV schema, so the same tools can point at the
hosted Jev API by setting `provider = "typesafe"` and `TYPESAFE_API_KEY`.

## License

MIT. Laya models are Apache-2.0 by Convai Innovations. laya.cpp is MIT by Lars
Karlslund. ohmylaya is an independent project and is not affiliated with
Convai Innovations, TypeSafe AI or the laya.cpp author.
