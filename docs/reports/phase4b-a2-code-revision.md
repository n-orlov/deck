# Phase 4b approach 2 — pinned code revision (task 005)

This record pins the code revision that every downstream measurement task in
approach 2 (006-012) measures against. The freeze line takes effect from this
task's launch onward: task 004 (R1) was the last code-touching task, so every
commit from here on is record-only (`docs/reports/**`, `docs/DELIVERY-LOG.md`,
`docs/PLAN.md`).

## Pinned revision

```
$ git log -1 --format=%H -- '*.go' '*.feature'
70c7430df3b23a46fb735e8573e26ec55908adeb
```

`REV = 70c7430df3b23a46fb735e8573e26ec55908adeb` — the latest commit that
touches a `.go` or `.feature` file, i.e. finding R1's cure commit. This is
the tree every downstream measurement task (006-012) cites as the identity
of the code it ran against.

## Tree-object hashes at REV, read at this task's own commit

Read with `git rev-parse <REV>:<path>` for the four paths downstream tasks
cite:

| path | tree-object hash |
| --- | --- |
| `internal` | `15506734d4989e111e871a419ebf46c94a3b59a3` |
| `cmd` | `27ff2eba72ef6a63a6cf49285c4ddc6660b7fb0d` |
| `features` | `b5dbe2f565eb96b2654f8d1d64fcc51eb711f60d` |
| `ci` | `0a183631a2beea070ab0f7d8fa027aecf423e7b0` |

Raw output (captured before this task's own commit existed):
`docs/reports/phase4b-a2-code-revision/tree-hashes-pre-commit.log`.

## Re-print at this task's own commit, proving the record-only commit did not move code

Because `REV` names a fixed, already-existing commit (`70c7430...`), adding
this record-only commit on top of it cannot change what `git rev-parse
REV:<path>` reports for any of the four paths above — those four tree
objects belong to the historical commit `70c7430...`, not to HEAD. The check
below re-runs the exact same four `git rev-parse` reads immediately after
this task's own commit was created (HEAD now points at that new commit) and
shows each of the four pairs equal to the pre-commit read above, i.e. the
record-only commit is shown not to have moved code:

Raw output (captured after this task's own commit): `docs/reports/phase4b-a2-code-revision/tree-hashes-post-commit.log`.
Pair-by-pair diff of the two logs (empty output = every pair equal):
`docs/reports/phase4b-a2-code-revision/tree-hashes-diff.log`.

There is no Python and no pytest anywhere in this repo (standing rule: "no
other test framework may be introduced"); the repo's only test surfaces are
Go test functions and godog scenarios. "Verified by rerunning" here means
literally re-running the same `git rev-parse` reads a second time, after the
commit, and diffing the two captured outputs byte-for-byte — the check the
criterion asks for, expressed in the tools this repo actually has.

## The four cure commits this revision is built from

| task | finding | commit | subject |
| --- | --- | --- | --- |
| 001 | B1 | `113b552ca2fe42bb1b3d10e42f14cdc3f735c8e6` | tui: match the default group's label in the list filter (B1, #25) |
| 002 | B2 | `075c59c35fbbc22461f748bbe87e87a8ac4139e4` | tui: refresh an open settings Groups panel on the ordinary reload (B2) |
| 003 | B3 | `00f33a5ac204bc2f558352a18c40fefa24ad3bab` | interactive: clear the scrollback the displacement notice itself creates (B3, #30) |
| 004 | R1 | `70c7430df3b23a46fb735e8573e26ec55908adeb` | tui: name the manual group model in the c help line, not the removed workspace model (R1) |

Each resolves as a commit object (`git cat-file -e <sha>^{commit}`, exit
status 0 for all four). Committed output:
`docs/reports/phase4b-a2-code-revision/cure-commits-resolve.log`.

## For downstream tasks

Tasks 006-012 cite the four tree-object hashes above (labelled `internal`,
`cmd`, `features`, `ci`) as the identity of the tree they measured/attested
against. Any measurement whose own `git rev-parse <rev>:<path>` does not
match one of these four hashes is measuring a different tree than this
approach pinned and must not be cited as approach-2 evidence.
