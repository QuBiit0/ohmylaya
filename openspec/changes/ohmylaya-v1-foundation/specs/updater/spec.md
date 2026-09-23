# Updater

Defines self-update and pinned dependency bumps.

## Requirements

### Requirement: Embedded manifest

The binary MUST embed a manifest listing, for each OS/arch/backend, the
laya.cpp release tag, asset URL, size and SHA-256, the Windows cuBLAS archive
URL and digests, and for each model the Hugging Face repository, revision and
the list of files with sizes and digests. `ohmylaya manifest` prints it.

#### Scenario: Reproducible stack

- GIVEN two machines with the same ohmylaya version
- WHEN both install
- THEN both end with byte-identical engine and checkpoint files.

### Requirement: Self-update

`ohmylaya update` MUST query GitHub releases, download the matching archive,
verify SHA-256 against `checksums.txt`, replace the running binary atomically
(rename on POSIX, rename-then-delete-on-next-start on Windows), and re-run
registrations so absolute paths stay valid. `--check` only reports.

#### Scenario: New release available

- GIVEN a newer release
- WHEN `ohmylaya update` runs
- THEN the binary is replaced, the skill is refreshed, and the summary lists engine or model bumps that the new manifest requires.

#### Scenario: Windows locked binary

- GIVEN Windows where the running executable cannot be overwritten
- WHEN update runs
- THEN the new binary is placed beside it and swapped on next start.

### Requirement: Engine and model bumps

After a self-update, or with `ohmylaya update --engine` and `--model`, the
tool MUST compare installed digests with the manifest and download only what
changed, then run the smoke test before switching the sidecar. The previous
engine MUST be kept as `bin/laya-cli.prev` until the smoke test passes.

#### Scenario: Engine bump fails smoke

- GIVEN a new engine that crashes on this GPU
- WHEN the smoke test fails
- THEN the previous engine is restored and the failure is reported with log lines.

### Requirement: Signing roadmap

v1 verifies SHA-256 from the release checksums file. The manifest format MUST
reserve a `signature` field so minisign verification can be added without a
breaking change.

#### Scenario: Future signed release

- GIVEN a release that includes a minisign signature
- WHEN an older v1 binary updates
- THEN it ignores the unknown field and still verifies SHA-256.
