# Installer

Defines how ohmylaya reaches a working local decision engine in one command.

## Requirements

### Requirement: One-shot bootstrap scripts

The project MUST publish `install.sh` (POSIX sh) and `install.ps1` (PowerShell 5.1+)
that download the ohmylaya binary for the current OS and architecture from the
latest GitHub release, verify its SHA-256 against the published checksums file,
place it in a user-writable bin directory, and then run `ohmylaya install`.

#### Scenario: Linux or macOS one-shot

- GIVEN a machine with `curl` or `wget` and no ohmylaya installed
- WHEN the user runs `curl -fsSL <raw install.sh url> | sh`
- THEN the script downloads the matching binary and checksums
- AND verifies the checksum before placing the binary in `~/.local/bin` (or `$OHMYLAYA_BIN_DIR`)
- AND prints the PATH line to add when the directory is not on PATH
- AND runs `ohmylaya install` in the same terminal.

#### Scenario: Windows one-shot

- GIVEN PowerShell 5.1 or newer
- WHEN the user runs `irm <raw install.ps1 url> | iex`
- THEN the script downloads and verifies the Windows binary
- AND places it in `%LOCALAPPDATA%\ohmylaya\bin`
- AND adds that directory to the user PATH if absent
- AND runs `ohmylaya install`.

#### Scenario: Checksum mismatch

- GIVEN a downloaded binary whose SHA-256 differs from the checksums file
- WHEN verification runs
- THEN the script deletes the download, prints the expected and actual digests, and exits non-zero without running anything.

### Requirement: Interactive and non-interactive install

`ohmylaya install` MUST run a guided flow on a terminal and MUST accept flags
that make every choice explicit so CI and scripts can run it without prompts.

Flags: `--backend auto|vulkan|cuda|cpu`, `--model multilingual|english|typed-decisions`,
`--agents claude,codex,opencode,pi|all|none`, `--yes`, `--home <dir>`, `--no-skill`, `--no-start`.

#### Scenario: Non-interactive install

- GIVEN `ohmylaya install --backend auto --model multilingual --agents claude,pi --yes`
- WHEN the command runs on a terminal or not
- THEN no prompt is shown
- AND the engine, checkpoint and registrations are applied
- AND the exit code is zero only when every selected step succeeded.

#### Scenario: Interactive install

- GIVEN `ohmylaya install` on an interactive terminal with no flags
- WHEN the command runs
- THEN the user is asked backend (with the detected recommendation preselected), model, and agents (detected agents preselected)
- AND a summary with download sizes is shown before any download starts.

### Requirement: Backend detection and recommendation

The installer MUST detect the platform and recommend a backend using the
following order, and MUST let the user override it.

| Platform | Detection | Recommended |
|---|---|---|
| Windows x64 | NVIDIA driver present (`nvcuda.dll` loadable) | `cuda` when the user opts in, otherwise `vulkan` |
| Windows x64 | `vulkan-1.dll` loadable | `vulkan` |
| Windows x64 | Neither | `cpu` |
| Linux x64 | `libcuda.so.1` present and glibc >= 2.39 | `cuda` when the user opts in, otherwise `vulkan` |
| Linux x64 | `libvulkan.so.1` present | `vulkan` |
| Linux x64 | Neither | `cpu` |
| macOS arm64 | Always | `cpu` in v1 |

The recommendation MUST show the download size of each option. CUDA on Windows
MUST also download the two cuBLAS DLLs listed in the manifest.

#### Scenario: NVIDIA machine on Windows

- GIVEN Windows with an NVIDIA driver
- WHEN detection runs
- THEN both `cuda` (about 200 MB plus cuBLAS DLLs) and `vulkan` (about 80 MB) are offered
- AND `vulkan` is preselected with a note that `cuda` is faster.

#### Scenario: No GPU

- GIVEN a machine without a usable GPU runtime
- WHEN detection runs
- THEN `cpu` is preselected
- AND the user is told expected latency is in the hundreds of milliseconds per question.

### Requirement: Verified, resumable, atomic downloads

Every download MUST be resumed when interrupted, verified against the manifest
digest after completion, and moved into place atomically. A file MUST never be
visible at its final path unless its digest matched.

#### Scenario: Interrupted checkpoint download

- GIVEN a checkpoint download interrupted at 40 percent
- WHEN `ohmylaya install` runs again
- THEN the download resumes from the partial file
- AND the final file is verified before placement.

#### Scenario: Corrupted download

- GIVEN a completed download with a wrong digest
- WHEN verification fails
- THEN the partial file is deleted, the error names the asset and both digests, and the install stops.

### Requirement: Home layout

The installer MUST create the following layout under `$OHMYLAYA_HOME`
(default `~/.ohmylaya`, Windows `%USERPROFILE%\.ohmylaya`):

```
bin/            laya-cli executable and Windows cuBLAS DLLs
models/laya/    checkpoint tree exactly as on Hugging Face
config.toml     user configuration
state/          sidecar.lock, sidecar.pid, sidecar.log, last-health.json
backups/        timestamped copies of every agent config file before a write
skills/         canonical copy of the installed skill
```

#### Scenario: Fresh install

- GIVEN no `$OHMYLAYA_HOME`
- WHEN install completes
- THEN the layout above exists with `config.toml` recording backend, model, port, idle timeout and registered agents.

### Requirement: Idempotent re-run

Running `ohmylaya install` again MUST skip assets whose digest already matches,
MUST re-apply registrations without duplicating entries, and MUST report what
changed.

#### Scenario: Second run

- GIVEN a completed install
- WHEN `ohmylaya install --yes` runs again
- THEN no download happens
- AND agent configs are byte-identical after the run
- AND the summary says "already up to date".

### Requirement: Verification smoke test

After placing the engine and checkpoint, the installer MUST start the sidecar,
send one fixed request, and confirm a structurally valid answer before
registering agents. With `--no-start` the smoke test is skipped and reported.

#### Scenario: Engine fails to start

- GIVEN a backend the machine cannot run
- WHEN the smoke test fails
- THEN the installer prints the engine's last log lines, suggests the next backend in the fallback order, and does not register agents.

### Requirement: Uninstall

`ohmylaya uninstall` MUST remove `$OHMYLAYA_HOME` and the `ohmylaya` entries
from each agent config it registered, restoring from backup when the file is
otherwise unchanged, and MUST leave every other setting intact. `--keep-models`
keeps the checkpoint tree.

#### Scenario: Uninstall with foreign edits

- GIVEN an agent config the user edited after registration
- WHEN uninstall runs
- THEN only the `ohmylaya` entry is removed and the user's edits are preserved.
