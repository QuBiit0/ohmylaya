# Sidecar runtime

Defines how ohmylaya runs and supervises the laya.cpp HTTP server.

## Requirements

### Requirement: Single shared sidecar

At most one sidecar per `$OHMYLAYA_HOME` MUST run at a time. Any ohmylaya
process that needs the engine (`mcp`, `serve`, `install`, `doctor --smoke`)
MUST reuse a healthy running sidecar and MUST start one only after acquiring
`state/sidecar.lock`.

#### Scenario: Two agents start at once

- GIVEN Claude Code and Codex both spawn `ohmylaya mcp` within the same second
- WHEN both need the engine
- THEN exactly one laya.cpp process starts
- AND both MCP servers report the same sidecar PID in their health metadata.

#### Scenario: Stale lock

- GIVEN a lock file whose PID is not alive
- WHEN a new process needs the engine
- THEN the stale lock is replaced and a sidecar starts normally.

### Requirement: Loopback port

The sidecar MUST bind `127.0.0.1` only. The default port is `45292`. When the
port is busy and not answering `/health` as a laya.cpp server, ohmylaya MUST
pick the next free port in `45292..45299`, record it in `state/`, and every
client MUST read the actual port from state instead of assuming the default.

#### Scenario: Port taken by another service

- GIVEN a foreign service on 45292
- WHEN the sidecar starts
- THEN it binds 45293
- AND `ohmylaya doctor` reports the port in use and the chosen alternative.

### Requirement: Launch arguments from configuration

The sidecar command line MUST be derived from `config.toml` as follows:

| Config | Argument |
|---|---|
| `backend = "cuda"` | `--cuda` and `--tensor-core-fp32 --flash-fp32` unless `precision.strict = true` |
| `backend = "vulkan"` | `--vulkan` and `--tensor-core-fp32` unless `precision.strict = true` |
| `backend = "cpu"` | `--cpu` |
| `model` | `--model <home>/models/laya --variant <model>` |
| `port` | `--server --host 127.0.0.1 --port <port>` |
| `batch.max_questions` (default 8) | `--max-questions N` |
| `batch.max_pending` (default 32) | `--max-pending-requests N` |
| `batch.wait_ms` (default 2) | `--batch-wait-ms N` |

Windows CUDA MUST run with `bin/` on the process PATH so the cuBLAS DLLs resolve.

#### Scenario: Strict precision

- GIVEN `precision.strict = true`
- WHEN the sidecar starts on CUDA
- THEN neither `--tensor-core-fp32` nor `--flash-fp32` is passed.

### Requirement: Readiness and health

Startup MUST wait for `GET /health` to report ready, with a timeout of 120
seconds by default (models load from disk). Health MUST be polled every 10
seconds while clients are attached and the last response cached in
`state/last-health.json`.

#### Scenario: Slow disk

- GIVEN a checkpoint that takes 40 seconds to load
- WHEN a client starts the sidecar
- THEN the client waits and reports progress instead of failing
- AND tools are advertised only after ready.

#### Scenario: Engine crash

- GIVEN a running sidecar that exits unexpectedly
- WHEN the next health poll fails
- THEN the supervisor restarts it once with backoff
- AND a second crash within five minutes stops restarts and surfaces the last 50 log lines through `doctor` and the MCP server's error hint.

### Requirement: Idle shutdown

The sidecar MUST stop after `idle_timeout` (default 30 minutes) with no requests
and no attached MCP clients. `idle_timeout = 0` disables idle stop. `ohmylaya
serve` keeps it alive regardless while that command runs.

On POSIX the supervisor MUST send SIGTERM and wait up to 10 seconds. On Windows
it MUST wait for in-flight batches (queue empty in health) for up to 10 seconds
and then terminate the process.

#### Scenario: Agent session ends

- GIVEN the last MCP client disconnects
- WHEN 30 minutes pass without requests
- THEN the sidecar exits and the lock and pid files are removed.

### Requirement: Logs

Sidecar stdout and stderr MUST be appended to `state/sidecar.log` with size
rotation at 5 MB keeping two generations. Request bodies MUST NOT be logged.

#### Scenario: Log rotation

- GIVEN a log above 5 MB
- WHEN a new line is written
- THEN the file rotates to `sidecar.log.1` and a fresh log starts.

### Requirement: Overload behaviour

When the sidecar answers 503 with `Retry-After`, ohmylaya clients MUST retry
with exponential backoff up to three attempts before returning a structured
error to the caller.

#### Scenario: Queue full

- GIVEN 40 concurrent tool calls
- WHEN the sidecar's pending limit is reached
- THEN excess calls wait and retry
- AND a call that still fails returns an error naming the queue limit and the `batch.max_pending` setting.
