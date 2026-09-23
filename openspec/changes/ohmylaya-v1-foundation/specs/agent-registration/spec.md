# Agent registration

Defines how ohmylaya registers its MCP server and skill in each supported agent
without depending on any agent framework.

## Requirements

### Requirement: Framework agnostic

ohmylaya MUST NOT import, call, read the configuration of, or require any
agent framework such as gentle-ai. Integration MUST use only two standard
surfaces: the agent's own MCP server registry and the agent's skill directory
with a `SKILL.md`. Any framework that reads those surfaces discovers ohmylaya
without changes on either side.

#### Scenario: Framework absent

- GIVEN a machine with Claude Code and no gentle-ai
- WHEN `ohmylaya install --agents claude` runs
- THEN registration succeeds and the tools appear in Claude Code.

#### Scenario: Framework updated

- GIVEN gentle-ai upgrades its internal formats
- WHEN ohmylaya runs unchanged
- THEN nothing breaks because ohmylaya never read gentle-ai files.

### Requirement: Adapter contract

Each agent MUST be implemented as an adapter with `Detect() (installed bool,
version string)`, `MCPConfigPath()`, `SkillDir()`, `Register(entry)`,
`Unregister()` and `Status()`. Adapters MUST be covered by golden tests that
start from real sample config files and assert the exact bytes after
registration and after unregistration.

#### Scenario: Golden round trip

- GIVEN a sample config with two unrelated servers
- WHEN register then unregister run
- THEN the final file equals the original byte for byte.

### Requirement: Safe writes

Every config write MUST: read the current file, refuse to proceed if it is not
parseable (never reset to an empty object), copy the original to
`$OHMYLAYA_HOME/backups/<agent>/<timestamp>-<name>`, merge only the `ohmylaya`
entry, write atomically via a temp file and rename, and preserve the original
file mode. JSON output MUST keep two-space indentation and key order of the
original where the parser allows.

#### Scenario: Corrupt config

- GIVEN an unparseable `~/.claude.json`
- WHEN registration runs
- THEN the adapter reports the parse error with the byte offset and changes nothing.

### Requirement: Claude Code adapter

Registers a user-scoped stdio server in `~/.claude.json` under `mcpServers.ohmylaya`
as `{"type":"stdio","command":"<abs path to ohmylaya>","args":["mcp"]}`.
When the `claude` CLI is on PATH the adapter MAY use `claude mcp add -s user`
and MUST verify the result by reading the file. Skill goes to
`~/.claude/skills/ohmylaya/SKILL.md`.

#### Scenario: Register in Claude Code

- GIVEN Claude Code detected
- WHEN registration runs
- THEN `~/.claude.json` contains the entry and the skill file exists.

### Requirement: Codex adapter

Registers `[mcp_servers.ohmylaya]` with `command` and `args = ["mcp"]` in
`~/.codex/config.toml`, preserving comments and other tables. Skill goes to
`~/.agents/skills/ohmylaya/SKILL.md`, the directory Codex scans, written as a
real directory and never a symlink.

#### Scenario: Register in Codex

- GIVEN `~/.codex/config.toml` with existing servers and comments
- WHEN registration runs
- THEN the new table is appended and every existing byte outside it is unchanged.

### Requirement: OpenCode adapter

Detects the config schema. For v1 configs it writes `mcp.ohmylaya = {"type":"local",
"command":["<abs path>","mcp"],"enabled":true}`. For v2 configs (servers under
`mcp.servers`) it writes `mcp.servers.ohmylaya` with the same shape without
`enabled`. Config path honours `XDG_CONFIG_HOME`, default
`~/.config/opencode/opencode.json` (or `.jsonc`). Skill goes to
`~/.config/opencode/skills/ohmylaya/SKILL.md`.

#### Scenario: OpenCode v2

- GIVEN a config that already has `mcp.servers`
- WHEN registration runs
- THEN the entry lands under `mcp.servers` and not at `mcp.ohmylaya`.

### Requirement: Pi adapter

Registers in `~/.pi/agent/mcp.json` under `mcpServers.ohmylaya` as
`{"command":"<abs path>","args":["mcp"],"lifecycle":"lazy"}`. This file belongs
to the `pi-mcp-adapter` package; when it is absent the adapter MUST report that
Pi has no MCP adapter installed and print the package name instead of creating
the file. Skill goes to `~/.agents/skills/ohmylaya/SKILL.md`, which Pi scans.

#### Scenario: Pi without MCP adapter

- GIVEN `~/.pi/agent` exists but `mcp.json` does not
- WHEN registration runs
- THEN the skill is installed, the MCP step is skipped, and the summary explains how to add `pi-mcp-adapter`.

### Requirement: Absolute command path

Registrations MUST use the absolute path of the ohmylaya binary that performed
the install, never a bare `ohmylaya`, so hosts with a filtered PATH still start
the server. `ohmylaya update` MUST rewrite entries when the binary moves.

#### Scenario: Host with minimal PATH

- GIVEN a host that spawns MCP servers without the user's shell PATH
- WHEN it starts `ohmylaya`
- THEN the absolute path resolves and the server starts.

### Requirement: Status reporting

`ohmylaya agents` MUST list every supported agent with detected, registered,
skill installed, and config path columns, and MUST exit non-zero when a
registered entry points to a missing binary.

#### Scenario: Moved binary

- GIVEN a registration pointing to a deleted path
- WHEN `ohmylaya agents` runs
- THEN the row shows "stale" and the fix command.
