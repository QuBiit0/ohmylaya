# TUI

Defines the interactive terminal interface.

## Requirements

### Requirement: Entry point

Running `ohmylaya` with no arguments on an interactive terminal MUST open the
TUI. On a non-interactive stdin it MUST print the status summary and exit.
Every action the TUI performs MUST also exist as a subcommand so nothing is
TUI-only.

#### Scenario: Piped invocation

- GIVEN `ohmylaya | cat`
- WHEN it runs
- THEN plain status text is printed and no TUI is drawn.

### Requirement: Screens

The TUI MUST provide: Status (engine, model, backend, GPU, sidecar state,
port, registered agents, versions), Setup (backend, model, agents; runs the
same flow as `install`), Agents (toggle registration per agent), Update
(check and apply), Doctor (live check list), Logs (tail of `sidecar.log`).
Navigation MUST work with arrow keys, `j`/`k`, `enter`, `esc`, and `q`.

#### Scenario: Change backend

- GIVEN the Setup screen
- WHEN the user selects `cuda` and confirms
- THEN downloads show progress, the smoke test runs, and Status reflects the new backend.

### Requirement: Long operations

Downloads and smoke tests MUST run asynchronously with progress bars and MUST
be cancellable with `esc`, leaving no partial files at final paths.

#### Scenario: Cancel download

- GIVEN a checkpoint download in progress
- WHEN the user presses `esc`
- THEN the partial file stays in the resume location and Status shows "model: incomplete, resume in Setup".

### Requirement: Visual language

Styling MUST use lipgloss with a single theme file, adapt to light and dark
terminals, and degrade to ASCII when the terminal lacks Unicode support.

#### Scenario: Minimal terminal

- GIVEN `TERM=dumb`
- WHEN the TUI starts
- THEN it falls back to the non-interactive status output.
