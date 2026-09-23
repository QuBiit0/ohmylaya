# Usage

## Layout

Everything lives under `~/.ohmylaya` (override with `OHMYLAYA_HOME`):

```
bin/            laya-cli executable, cuBLAS DLLs on Windows CUDA
models/laya/    checkpoint tree, same layout as the Hugging Face repository
config.toml     your configuration
state/          sidecar.json, sidecar.lock, sidecar.log, clients/
backups/        every agent config file before ohmylaya wrote it
skills/         canonical copy of the installed skill
```

## config.toml

```toml
version = 1
backend = "vulkan"          # vulkan | cuda | cpu
model = "multilingual"      # multilingual | english | typed-decisions
port = 45292                # loopback only; 45293..45299 are tried when busy
idle_timeout = "30m"        # "0s" keeps the engine alive
provider = "local"          # local | typesafe
log_level = "info"

[precision]
strict = false              # true disables the faster tensor-core paths

[batch]
max_questions = 8           # per engine call; larger requests are split
max_pending = 32            # queued HTTP calls before the engine returns 503
wait_ms = 2

[tools]
auto_accept = 0.8           # action=auto at or above this gating value

[agents]
registered = ["claude", "pi"]
```

Environment overrides: `OHMYLAYA_PORT`, `OHMYLAYA_PROVIDER`,
`OHMYLAYA_LOG_LEVEL`, `TYPESAFE_API_KEY`.

## Commands

| Command | What it does |
|---|---|
| `ohmylaya` | TUI on a terminal; plain status when piped |
| `ohmylaya install [flags]` | download, verify, smoke test, register; safe to re-run |
| `ohmylaya uninstall [--keep-models]` | remove everything ohmylaya wrote |
| `ohmylaya agents [--json] [--register a,b] [--unregister a,b]` | status and toggles |
| `ohmylaya doctor [--json] [--smoke] [--fail-on warn]` | diagnostics |
| `ohmylaya update [--check]` | self-update, then reconcile engine, model, skill |
| `ohmylaya mcp` | MCP server on stdio; agents run this |
| `ohmylaya ask <tool>` | call one tool with JSON on stdin |
| `ohmylaya version` | version |

Install flags: `--backend auto|vulkan|cuda|cpu`, `--model`, `--agents
claude,codex,opencode,pi|all|none`, `--yes`, `--no-skill`, `--no-start`.

## The engine lifecycle

The first tool call starts `laya-cli` as a detached process, waits for
`/health`, and records it in `state/sidecar.json`. Every ohmylaya process
that needs it attaches to the same engine; a lock file prevents double
starts. The engine stops after `idle_timeout` with no requests and no
attached clients, or when the configuration it was started with changes.
Windows terminates it after draining the queue; POSIX sends SIGTERM.

The first inference after a start pays a shader warmup of a few seconds on
Vulkan. The install smoke test absorbs it.

## Calling tools from scripts

```sh
echo '{"query":"where is the port chosen?","glob":"internal/**/*.go","top_k":5}' | ohmylaya ask rerank
echo '{"ref":{"url":"https://example.com"},"purpose":"find pricing"}' | ohmylaya ask screen
```

Each result carries `meta.provider`, `meta.model`, `meta.truncated`,
`meta.warnings` and `meta.elapsed_ms`.

## Using it from your own agent

Three doors, all framework-free:

1. MCP over stdio: run `ohmylaya mcp` from any MCP client.
2. HTTP: the engine serves the JEV schema on `127.0.0.1:<port>`;
   `POST /v1/systemone` for one request, `POST /predict` for a batch. Read
   the port from `state/sidecar.json`.
3. CLI: `ohmylaya ask` from hooks, CI and pipelines, no model tokens spent.
