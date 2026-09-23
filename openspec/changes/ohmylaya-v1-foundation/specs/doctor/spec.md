# Doctor

Defines diagnostics.

## Requirements

### Requirement: Check catalogue

`ohmylaya doctor` MUST run these checks and print one line per check with
PASS, WARN, FAIL or SKIP, a short reason, and a fix command when failing:

| Check | What it verifies |
|---|---|
| platform | OS, arch, glibc version on Linux, macOS version |
| home | `$OHMYLAYA_HOME` layout and config parse |
| engine | executable present, digest matches manifest, runs `--help` |
| runtime-deps | Vulkan loader, NVIDIA driver, cuBLAS DLLs as required by backend |
| gpu | device name reported by the engine on startup, or CPU |
| model | checkpoint files present with expected sizes and digests |
| port | configured port free or owned by a healthy sidecar |
| sidecar | `/health` ready, loaded model, backend, queue counts |
| smoke (with `--smoke`) | one fixed request returns valid probabilities |
| agents | per agent: detected, registered, absolute path valid, skill version |
| version | binary up to date against GitHub releases (skipped offline) |

#### Scenario: Missing cuBLAS DLLs

- GIVEN backend `cuda` on Windows without the DLLs
- WHEN doctor runs
- THEN `runtime-deps` FAILS naming both DLL files and the `ohmylaya install --backend cuda` command.

#### Scenario: All good

- GIVEN a healthy install
- WHEN doctor runs
- THEN every check PASSES and the exit code is zero.

### Requirement: Machine-readable output

`--json` MUST print a single JSON object with `checks[]`, `summary`, `version`,
`home` and `config` (secrets redacted), suitable for pasting into an issue.
`--fail-on warn` MUST make WARN exit non-zero.

#### Scenario: Bug report

- GIVEN a user opening an issue
- WHEN they run `ohmylaya doctor --json`
- THEN the output contains no API keys and enough context to reproduce.

### Requirement: Offline behaviour

Network checks MUST time out in three seconds and report SKIP, never FAIL,
when the network is unreachable.

#### Scenario: Offline machine

- GIVEN no network
- WHEN doctor runs
- THEN `version` is SKIP and the exit code reflects only local checks.
