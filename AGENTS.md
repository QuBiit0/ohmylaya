# AGENTS.md

Instructions for AI coding agents working in this repository.

## Read first

- `openspec/project.md` for purpose, stack and conventions.
- `openspec/changes/<change>/` for the active proposal, specs, design and tasks.
- `docs/research/` for the evidence behind design decisions.

## Rules

- Everything in this repository is written in English: code, comments, docs,
  commit messages, UI strings.
- No Go file is written without a failing test first. `go test -race ./...`
  must pass before a commit.
- One task from `tasks.md` per pull request, under 400 changed lines. Split
  instead of stretching.
- Never write outside `$OHMYLAYA_HOME` and the agent config files the user
  selected. Merge, back up, write atomically, never reset an unparseable file.
- ohmylaya must stay independent of every agent framework. Do not read or
  write gentle-ai files or any framework-specific state. Only MCP registries
  and skill directories are integration surfaces.
- Do not add telemetry or network calls beyond the documented downloads and
  the optional hosted provider.
- Keep the honest limits of the model visible in README, tool descriptions and
  the skill. Do not soften them.

## Layout

See design D13 in the active change for the intended package layout.
