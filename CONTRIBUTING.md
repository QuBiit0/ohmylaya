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
  turns green. `go test -race ./...` must pass; CI runs it on Linux, macOS
  and Windows.
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

Requires Go (version in `go.mod`). `-race` needs a C toolchain; without one,
run `go test ./...` and let CI run the race detector.

```sh
make test                  # go test -race ./...
make lint                  # go vet and staticcheck
make build                 # bin/ohmylaya
./bin/ohmylaya version
```

| Also useful | Command |
|---|---|
| Tests against the real engine | `OHMYLAYA_TEST_ENGINE=1 go test -tags engine ./internal/jev/` |
| Benchmark the tools offline | `go run ./bench -bin bin/ohmylaya` (see `docs/benchmarks.md`) |
| Refresh the engine and model manifest | `make manifest TAG=<laya.cpp tag> REVISION=main` |
| Install a release on clean CI runners | `gh workflow run install-smoke.yml -f version=v0.x.y` |

Try changes without touching your real install by pointing `OHMYLAYA_HOME`
and `OHMYLAYA_PORT` at a scratch directory and a free port.

## Releasing

1. Add the release to `CHANGELOG.md`.
2. Tag it: `git tag -a v0.x.y -m "ohmylaya v0.x.y"`, then push the tag.
3. The release workflow runs the tests, publishes with GoReleaser, then runs
   `install-smoke` on Linux, macOS and Windows. Check that it is green.
