# Contributing

Thanks for helping build ohmylaya.

## Before you write code

- Read `AGENTS.md`, `openspec/project.md` and the active change under
  `openspec/changes/`. The specs are the contract.
- Open or find an issue first. Pull requests without a linked issue are
  closed with a pointer to this file.
- Pick one task from `tasks.md` per pull request.

## Rules

- English everywhere: code, comments, docs, commits, UI strings.
- Test first. Every behaviour change ships with a failing test that the change
  turns green. `go test -race ./...` must pass locally.
- Pull requests stay under 400 changed lines, excluding `testdata/`, `docs/`
  and `openspec/`. CI enforces it. Chain PRs when a task needs more.
- Conventional commit prefixes: `feat`, `fix`, `docs`, `test`, `refactor`,
  `chore`, `ci`.
- Never write outside `$OHMYLAYA_HOME` and the selected agent config files.
  Never read or write another framework's files.
- Keep the model's honest limits visible. Marketing copy that hides them is a
  bug.

## AI assistance

See `AI_POLICY.md`. You are responsible for what you submit, whichever tool
helped you write it.

## Local setup

```sh
go test -race ./...
make build
./bin/ohmylaya version
```

Integration tests that need the real engine are behind the `engine` build tag
and `OHMYLAYA_TEST_ENGINE=1`.
