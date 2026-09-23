# Engine verification (task T0)

Date: 2026-09-23. Engine: laya.cpp `r0002`. Checkpoint: `convaiinnovations/laya`
revision `1c5edc17a7acd8701df6fc341c0d179f1c62c982`, variant `multilingual`.

## Windows x64 (verified)

Machine: Windows 11, AMD Ryzen 7 7435HS, NVIDIA GeForce RTX 3050 Laptop GPU.

| Item | Result |
|---|---|
| `laya-r0002-windows-amd64-vulkan.exe` digest | matches `SHA256SUMS` (`dcaf474b...9728`) |
| `multilingual/model.safetensors` digest | matches Hugging Face LFS oid (`9d628fd9...f204`) |
| Files needed per variant | `model.safetensors`, `rl_agent_config.json`, `encoder/config.json`, `tokenizer/tokenizer.json`, `tokenizer/tokenizer_config.json` |
| `--help` on the Vulkan build | exit 0, lists `--cpu`, `--vulkan`, `--server`, queue flags |
| `--cpu --server` on the Vulkan build | ready in 4 s, `/health` reports `backend: "CPU"` |
| `--vulkan --tensor-core-fp32 --server` | ready in 4 s, `/health` reports `Vulkan0`, log names the RTX 3050 |
| CPU latency, 3 questions, one request | 0.47 to 0.57 s |
| Vulkan latency, 3 questions, one request | 44 to 52 ms |
| CPU and Vulkan answers | identical to four decimals on the smoke requests |
| Spanish state on `multilingual` | refund `noul` 0.977 |
| Prompt-injection text, `noul` "instructions aimed at an AI agent" | 0.9994 |
| Two-option `choice` with 0.61 / 0.39 split | `confidence` 0.0351 (normalised entropy, not the top probability) |
| Invalid question type | HTTP 422 `{"error":{"message":"Unsupported question type: bogus","status":422}}` |
| `GET /v1/models` | one entry, `laya-multilingual`, release date `2026-09-20` |
| Response headers | `X-Laya-Batch-Id`, `X-Laya-Batch-Offset` present on every 200 |
| Stop | `taskkill /F` ends the process; `/health` unreachable within 1 s |

Recorded responses live in `testdata/engine/r0002-multilingual/` and feed the
golden tests in `internal/jev` and `internal/tools`.

Observations that change or confirm the design:

- The Vulkan executable is a valid CPU fallback on Windows even though this
  machine has a Vulkan loader; a loader-less machine still needs a check. The
  executable imports `vulkan-1.dll` only through ggml's loader, so a missing
  DLL is expected to fail at process start. Doctor must probe for the DLL
  before choosing `vulkan` and fall back to the CUDA build or report clearly.
- `confidence` is normalised entropy. A confident two-way split still reads
  low. The tools' `auto_accept` threshold must compare against the top
  probability for two-option questions (`check`), and against `confidence`
  for multi-option ones. Spec `mcp-tools` is updated accordingly.
- `action.act_probability` reads 1.0 everywhere, as upstream documents.
  Ignore it.
- `/predict` accepts an `id` per request and returns results in order with
  `model: "laya-rl-agent"`, while `/v1/systemone` returns `laya-multilingual`.
  The local provider should use `/predict` for batching and normalise the
  model name from `/health`.
- The 16-request burst in this spike ran sequentially by mistake (about
  150 ms each including header capture). Real concurrency and the 503 path are
  covered by the fake-engine tests in T6 and the `engine` tagged integration
  tests.

## Linux x64 (pending)

Needs a machine or CI runner with glibc 2.39+. Verify the Vulkan executable
with `--cpu` on a host without `libvulkan.so.1`, then with a GPU.

## macOS arm64 (pending)

Needs Apple Silicon. Verify `laya-r0002-macos-arm64-coreml` with `--cpu` and
no `coreml/` buckets present. If it refuses to start, ask upstream for a CPU
build or add `--cpu` handling before Core ML initialisation.
