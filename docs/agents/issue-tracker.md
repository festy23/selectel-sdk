# Issue tracker: GitHub

Specifications and tasks live in festy23/selectel-sdk GitHub Issues.
Use gh with an explicit --repo festy23/selectel-sdk.

## Operations

- Publish: create a GitHub issue with gh issue create.
- Fetch: gh issue view <number> --comments; also inspect its labels.
- Search: gh issue list with appropriate state, search and label filters.
- Comment: gh issue comment <number>.
- Change labels or assignees: gh issue edit <number>.
- Resolve: record the outcome and close with gh issue close.

For multiline issue bodies and comments, write the text to a temporary
file and use --body-file. Before publishing, check for an existing
issue for the same specification or task.

## Pull requests as a triage surface

PRs as a request surface: no.

## Wayfinding

The map is an issue labelled wayfinder:map. Tickets are child issues,
labelled wayfinder:research, wayfinder:prototype, wayfinder:grilling,
or wayfinder:task.

Use native GitHub sub-issues and issue dependencies where available.
Otherwise, maintain a task list in the map, Part of #<map> in each
child, and Blocked by: #<number> references for dependencies.

The frontier consists of open, unassigned children with no open
blockers, in map order. Claim a ticket by assigning it to @me.
Resolve by recording the result, closing the ticket, and adding a
brief result with its link to the map's Decisions-so-far.
