# Agent skill

Defines the `ohmylaya` skill that teaches agents when and how to use the tools.

## Requirements

### Requirement: Skill format

The skill MUST be a directory `ohmylaya/` with `SKILL.md` whose frontmatter has
`name: ohmylaya`, a `description` that starts with `Trigger:` and lists trigger
words (ohmylaya, laya, typed decision, screen content, rerank, classify, check
claim, calibrated confidence), `license: MIT`, and `metadata.version` equal to
the binary version. The body MUST stay under 700 tokens. Long material goes in
`references/` files loaded on demand.

#### Scenario: Registry indexes the skill

- GIVEN the skill installed in `~/.agents/skills/ohmylaya`
- WHEN a skill registry scans the directory
- THEN the name and triggers are picked up from the frontmatter alone.

### Requirement: Content contract

The body MUST contain, in this order: what Laya is in two sentences, a table
mapping situations to tools, the confidence rule (act on `auto`, escalate or
ask on `review`), the honest limits (near chance zero-shot on domain decisions,
weak ordinal scores, more than 20 options, state truncated from the end, never
use it to approve code), and how to recover when tools are missing
(`ohmylaya doctor`).

#### Scenario: Agent picks the right tool

- GIVEN an agent about to read a fetched web page
- WHEN it consults the skill
- THEN the table points it to `screen` with the page text and its purpose.

#### Scenario: Agent respects limits

- GIVEN an agent asked to approve a pull request
- WHEN it consults the skill
- THEN the skill tells it not to use ohmylaya as the approval authority.

### Requirement: References

`references/questions.md` MUST show how to write good `instructions` and
`criteria` for each primitive, with one worked example per tool.
`references/thresholds.md` MUST explain confidence, `auto_accept`, and why
raw probabilities are over-confident until calibrated on the user's data.

#### Scenario: Agent needs a better question

- GIVEN a `classify` result full of `review` actions
- WHEN the agent opens `references/questions.md`
- THEN it finds guidance on descriptive label criteria and shorter, single-topic items.

### Requirement: Embedded and versioned

The skill MUST be embedded in the binary and written by `install` and `update`.
`doctor` MUST flag a skill whose `metadata.version` differs from the binary.

#### Scenario: Outdated skill

- GIVEN a skill from an older release
- WHEN `doctor` runs
- THEN it reports "skill outdated" with the update command.
