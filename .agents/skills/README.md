# Matt Pocock skills

This directory vendors all 38 skills from [mattpocock/skills](https://github.com/mattpocock/skills) at revision `59db609eb1e18aaf59b63284d0e52255aff0c63c`, the checkout used by plugin version 1.3.1.

Each skill lives in `<name>/SKILL.md`, alongside its upstream references, templates, scripts, and Codex metadata. These files are committed directly to the repo so cloud and local Codex sessions can discover them through `.agents/skills`, without downloading the plugin at runtime. Invoke a skill by its name, for example `$ask-matt` or `$tdd`. Skills marked for explicit invocation retain that policy in `agents/openai.yaml`.

## Included skills

The published plugin contains the engineering and productivity groups. The miscellaneous and in-progress groups are also included to preserve the full upstream collection.

| Upstream group | Skills |
| --- | --- |
| Engineering (20) | ask-matt, code-review, codebase-design, diagnosing-bugs, domain-modeling, grill-with-docs, implement, implement-spec, improve-codebase-architecture, pr, prototype, research, retro, setup-matt-pocock-skills, tdd, to-spec, to-tickets, triage, wayfinder, wizard |
| Productivity (7) | grill-me, grilling, handoff, teach, to-questionnaire, wait-what, writing-for-agents |
| Miscellaneous (4) | git-guardrails-claude-code, migrate-to-shoehorn, scaffold-exercises, setup-pre-commit |
| In progress (7) | chief-of-staff, claude-handoff, loop-me, setup-ts-deep-modules, writing-beats, writing-fragments, writing-shape |

## Environment requirements

The skill files and support files are unmodified upstream copies. Individual workflows still need the tools and credentials they call, such as authenticated `gh` access for GitHub operations. Claude-specific skills require Claude tooling; vendoring them does not install that tooling or activate hooks. In-progress skills retain their upstream experimental status.

Repo-specific tracker, triage-label, and domain conventions live in `docs/agents/`, as directed by the root `AGENTS.md`.

## Repository skills

The following skills are maintained locally, outside the upstream collection:

- [polish](polish/SKILL.md) runs delegated review, validation, and fixes after implementation for the user's requested number of rounds, defaulting to three. Requests to polish an implementation or run iterative review-and-fix passes trigger it automatically. Round 1 covers the full implementation; subsequent rounds cover only additional changes.

## Updating

Choose an upstream revision, then use Codex's `skill-installer` helper with `--repo mattpocock/skills --ref <revision> --dest <staging-directory> --path <upstream-skill-paths>`. Install every directory under upstream `skills/` that contains a `SKILL.md`; copy each complete skill folder here by its basename. Preserve supporting files and the upstream license, and update this revision and inventory. Review changes before replacing the checked-in copies.

The upstream skills are distributed under the [MIT license](LICENSE), copyright 2026 Matt Pocock.
