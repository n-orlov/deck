# Phase 3f: fixing shas posted to GitHub issues #5–#10 (task 026)

Each of the six field defects reported as GitHub issues on `n-orlov/deck` received one
comment naming the requirement it became, the fixing sha(s), the regression tests, and the
revert-and-reproduce proof. Posted through the GitHub REST API
(`POST /repos/n-orlov/deck/issues/{n}/comments`) with the token in `~/.git-credentials`.

**No issue was closed, and this record does not authorise closing any of them.** The six
issues were all `open` before and after; state was re-read per issue after posting (table
below). Every comment says so in its own last line, and no comment contains a GitHub
closing keyword (`closes #N` / `fixes #N` / `resolved #N` …) — scanned, `CLEAN` for all six.

## Issue → requirement → fixing shas, with the API response

| issue | requirement | fixing sha(s) | POST status | comment id | state after |
|---|---|---|---|---|---|
| #5 | R68 — a terminal query can never stall an interactive session | `f3c25d5`, `7d060cd` | **201** | 5432079186 | open (2 comments) |
| #6 | R69 — a retained dead pane is collected on sight | `b8f2513`, `366dd78` | **201** | 5432079373 | open (1 comment) |
| #7 | R73 — the three scrollable overlays scroll by line and by wheel | `2714d1b`, `4edbfc2`, `9c2e66a` | **201** | 5432079694 | open (1 comment) |
| #8 | R71 — archived rows refuse launches and can be unarchived | `88742b2`, `63d4189`, `9d43a32` | **201** | 5432079788 | open (3 comments) |
| #9 | R70 — a resume clears the replaced pane's crash verdict | `0745ced` | **201** | 5432079863 | open (2 comments) |
| #10 | R72 — `A` confirms before it archives, and `u` undoes it | `10f3970`, `4822484`, `eb2089e` | **201** | 5432079953 | open (1 comment) |

All six comments were created `2026-08-26T22:59:14Z`–`22:59:20Z` by `n-orlov`. Independent
verification is a `curl` of each issue and each issue's `/comments` collection *after* the
posts (a second code path from the one that wrote them): the newest comment on every issue
is the one posted here, and every issue still reads `state: open`.

Each comment carries the phase's two deliverable runs as context, unchanged from the
report: whole suite green at `e47cb35`, ten-run stability **9/10** at `c12c30e`, with the
single failure named as `preview.feature:134`'s pre-resize fit race — R65's residual, which
phase 3f records as a **FAILED** requirement and does not claim fixed. No comment claims
10/10.

## Issue #8's comments were read before R71 was implemented, and the correction was followed

The task required confirming that issue #8's **comment thread**, not only its body, was
read, and that the published correction is reflected in the delivered work. Both comments
were fetched and read this iteration (`GET /repos/n-orlov/deck/issues/8/comments`, ids
`5425310900` of 2026-08-26T12:29:43Z and `5425368794` of 12:34:54Z) and checked against the
shipped code:

1. **The design decision that replaced the reporter's own suggestion** (comment
   `5425368794`): clearing `archived_at` on resume was withdrawn in favour of *"an archived
   session must not be startable at all"* plus an explicit UI unarchive. That is what
   shipped — `88742b2` **refuses** `ArchivedAt != 0` before the launch lease rather than
   clearing the flag, and `63d4189` adds `Store.UnarchiveSession` (modelled on
   `RestoreSession`) bound to `U` in the filter results. The comment's note that
   `internal/store/store.go`'s "an archived row has no restore at all" documentation is
   *part of* the change, not collateral, was honoured in `63d4189`.
2. **The evidence correction in the same comment**: `last_probe_at` is not support for
   severed reconciliation, because `RecordProbeMiss` is its only writer. Nothing in
   `docs/reports/phase3f.md` cites `last_probe_at` as evidence; R71's severed-reconcile
   reasoning rests on control flow at `internal/service/reconcile.go:48`. The comment says
   the same correction applies to #9, and #9's own comment here repeats it.
3. **The severity/priority point** (comment `5425310900`): the injected-identity route
   failed on the *same* archived-excluding slice, so no key could have resolved the row —
   which is why leg 3 (`9d43a32`) reads `ListSessionsIncludingArchived` and removes the
   class, and why it was kept even though leg 1 makes the reported state unreachable.

## Artifacts

Under the run's artifacts directory, `task026-issue-comments/`: the six comment bodies as
posted (`issue-05-r68.md`, `issue-06-r69.md`, `issue-07-r73.md`, `issue-08-r71.md`,
`issue-09-r70.md`, `issue-10-r72.md`), the API POST log with the six 201s
(`api-post-responses.log`), and the post-hoc `curl` verification
(`curl-verification.log`). The token was never printed, never passed as a command
argument, and the temporary `curl` config holding the auth header was created mode 600 and
deleted (verified absent).
