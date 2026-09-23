# Troubleshooting

Run `ohmylaya doctor` first. Each failing line has a fix. Paste
`ohmylaya doctor --json` into bug reports; it contains no secrets.

| Symptom | Cause | Fix |
|---|---|---|
| Agent lists no ohmylaya tools | not registered, or the agent was not restarted | `ohmylaya agents`, then restart the agent |
| Tools appear but every call errors with "engine unavailable" | engine cannot start on this backend | read the log lines in the error, then `ohmylaya install --backend cpu` |
| `runtime-deps FAIL vulkan-1.dll not found` | no Vulkan-capable driver | install the GPU driver or switch to `cpu` |
| `runtime-deps FAIL missing cublas64_13.dll` | CUDA chosen but DLLs absent | `ohmylaya install --backend cuda` |
| First call takes many seconds | shader warmup after a cold start | expected once per engine start |
| `port WARN in use by another service` | something else on 45292 | harmless; the engine takes the next port. Set `port` in config to silence it |
| `agents FAIL stale` | binary moved or reinstalled elsewhere | `ohmylaya install` rewrites the absolute path |
| Pi shows "no mcp.json" | Pi has no MCP adapter | install `pi-mcp-adapter` in Pi, then `ohmylaya agents --register pi` |
| `screen` blocks a README | base model over-reads imperative docs as injection | treat `block` as inspect; tune `thresholds` per call |
| `meta.truncated: true` | state longer than the checkpoint budget | chunk or summarise; the tail was cut |
| 503 or "overloaded" under load | more than `batch.max_pending` calls queued | raise it in config, or reduce concurrency |
| Linux: engine exits immediately | glibc older than 2.39 | upstream builds need Ubuntu 24.04 or newer |

Logs: `~/.ohmylaya/state/sidecar.log` (rotates at 5 MB). The TUI's Logs
screen tails it.

Uninstall and start over: `ohmylaya uninstall`, then run the one-shot again.
