# Troubleshooting

Run `ohmylaya doctor` first. Each failing line has a fix. Paste
`ohmylaya doctor --json` into bug reports; it contains no secrets.

| Symptom | Cause | Fix |
|---|---|---|
| Agent lists no ohmylaya tools | not registered, or the agent was not restarted | `ohmylaya agents`, then restart the agent |
| Install stops with "the engine needs the Vulkan loader" | no GPU driver, so no Vulkan loader; the CPU backend needs it too | Windows: install your GPU driver or the [Vulkan Runtime](https://vulkan.lunarg.com/sdk/home). Linux: `sudo apt install libvulkan1` or your distribution's `vulkan-loader` |
| Install stops with "needs macOS 15 or newer" | the upstream engine targets macOS 15 | update macOS; there is no build for older versions |
| Smoke test fails on `vulkan` on a machine without a GPU | the Vulkan loader is installed but there is no GPU device | `ohmylaya install --backend cpu` |
| Tools appear but every call errors with "engine unavailable" | engine cannot start on this backend | read the log lines in the error and run `ohmylaya doctor` |
| `runtime-deps FAIL vulkan-1.dll not found` or `libvulkan.so.1 not found` | Vulkan loader missing (driver removed or never installed) | same fix as the "needs the Vulkan loader" row above; switching to `cpu` does not help |
| `runtime-deps FAIL missing cublas64_13.dll` | CUDA chosen but DLLs absent | `ohmylaya install --backend cuda`; v0.1.0 and v0.1.1 could not extract them, so update first |
| First call takes many seconds | shader warmup after a cold start | expected once per engine start |
| `port WARN in use by another service` | something else on 45292 | harmless; the engine takes the next port. Set `port` in config to silence it |
| `agents FAIL stale` | binary moved or reinstalled elsewhere | `ohmylaya install` rewrites the absolute path |
| Pi shows "no mcp.json" | Pi has no MCP adapter | install `pi-mcp-adapter` in Pi, then `ohmylaya agents --register pi` |
| `screen` blocks a README | `screen` reads imperative docs as injection (45% accuracy on the benchmark) | treat `block` as inspect and read the page yourself; tune `thresholds` per call |
| Answers look random | `multilingual` checkpoint on English content | `ohmylaya install --model english`; see [benchmarks](benchmarks.md) |
| `meta.truncated: true` | state longer than the checkpoint budget | chunk or summarise; the tail was cut |
| 503 or "overloaded" under load | more than `batch.max_pending` calls queued | raise it in config, or reduce concurrency |
| Linux: install stops with "need glibc 2.39 or newer" | upstream engine builds need glibc 2.39 | use Ubuntu 24.04 or another distribution with glibc 2.39+ |

Logs: `~/.ohmylaya/state/sidecar.log` (rotates at 5 MB). The TUI's Logs
screen tails it.

Uninstall and start over: `ohmylaya uninstall`, then run the one-shot again.
