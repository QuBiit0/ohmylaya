# Security policy

## Reporting a vulnerability

Please do not open a public issue. Report it privately through
[GitHub security advisories](https://github.com/QuBiit0/ohmylaya/security/advisories/new).
Include the version (`ohmylaya version`), your OS, and steps to reproduce.

You will get an answer within a week. Fixes ship in a patch release, and the
advisory credits you unless you ask otherwise.

## Supported versions

Only the latest release gets fixes. `ohmylaya update` moves you to it.

## What ohmylaya trusts

- **Downloads.** The installer checks the `ohmylaya` binary against the
  release `checksums.txt`. Engine and model files are checked against the
  SHA-256 digests in the manifest embedded in the binary.
- **Network.** The engine listens on `127.0.0.1` only. Outbound traffic is
  limited to these downloads, the update check against GitHub releases, the
  `url` references you pass to a tool, and the hosted provider if you enable
  it.
- **Files.** ohmylaya writes only under `~/.ohmylaya`, to the bin directory
  the one-shot script installs it into, and to the MCP registry and skill
  directory of each agent you select (listed in the README). It backs up
  every agent config file before writing.
- **Content.** Tools read the paths and URLs you pass. With the default
  `local` provider their content goes only to the local engine. With
  `provider = "typesafe"` it is sent to the hosted Jev API, so do not enable
  hosted mode for content that must stay on your machine.
- **`screen` is not a security control.** It is a weak hint, not a
  prompt-injection defence: see [docs/limits.md](docs/limits.md).
