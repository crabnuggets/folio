# Issue tracker: GitHub

Issues and specs live in crabnuggets/folio on GitHub. Run the
`gh` CLI from this clone; it infers the repository from origin.

## Operations

- Create: `gh issue create --title "..." --body-file <file>`
- Read: `gh issue view <number> --json number,title,body,labels,comments`
- List: `gh issue list --state open --json number,title,body,labels,assignees`
- Comment: `gh issue comment <number> --body-file <file>`
- Label: `gh issue edit <number> --add-label "<label>"`
- Remove a label: `gh issue edit <number> --remove-label "<label>"`
- Claim: `gh issue edit <number> --add-assignee "@me"`
- Close: `gh issue close <number> --comment "..."`

For multiline bodies, write the exact Markdown to a temporary file
and pass it with --body-file.

When a skill says "publish to the issue tracker", create a GitHub issue.
When it says "fetch the relevant ticket", read the GitHub issue.

## Pull requests as a triage surface

**PRs as a request surface: no.**

## Wayfinding

The map is an issue labelled `wayfinder:map`, containing Notes,
Decisions-so-far, and Fog. Child tickets use `wayfinder:<type>`,
where type is research, prototype, grilling, or task.

Link children with GitHub sub-issues when supported. Otherwise,
list them in the map's task list and begin each child body with
`Part of #<map>`.

Record blockers using native GitHub issue dependencies when supported.
Otherwise, begin the child body with `Blocked by: #<number>, ...`.
A ticket is unblocked when every blocker is closed.

Choose the first open, unassigned, unblocked child in map order.
Claim it by assigning yourself. On resolution, comment with the answer,
close the ticket, and append a summary and link to Decisions-so-far.
