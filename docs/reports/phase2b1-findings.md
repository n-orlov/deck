# Phase 2b-1 findings

This file accumulates findings across Phase 2b-1's tasks. It is written to
incrementally by task 013 (below) and by later tasks (018, 020, 023, 024, 033,
034 in `SPEC.md`/PRD's own numbering); each task adds its own dated section
rather than overwriting earlier ones.

## Task 013: re-aiming negative assertions whose copy moved to the footer

### The sweep

Task 012 moved a session row's status *reason* text (` · resumable`,
`starting · awaiting signal`) off the row and onto the footer
(`Model.selectedRowReason()`, `internal/tui/tui.go`). A repo-wide sweep for
negative screen-text assertions was run to find every assertion at risk of
going vacuous because the copy it polices moved surfaces:

```
grep -rn "does not contain" features/*.feature
grep -rn "!strings.Contains(view\|!strings.Contains(frame\|!strings.Contains(detail\|strings.Contains(view, \"" \
  internal/tui/*_test.go internal/tui/*.go features/*.go cmd/deck/*_test.go
```

Findings, by site:

| Site | Copy | Scope of the check | At risk from task 012? |
| --- | --- | --- | --- |
| `features/shell_liveness.feature:10` | `awaiting signal` | `deck client "A" screen does not contain ...` — whole rendered frame | Yes — this is the copy that moved. |
| `features/launch_lease.feature:20,32,46,57` | `starting elsewhere` | `deck client "A" screen does not contain ...` — whole rendered frame | Named directly in the PRD table for this task. |
| `features/lease_race.feature:30-32` | `running` | `row "unsignalled agent" does not contain ...` — scoped to one row | No — `running` is a bare status word, never a reason; still lives on the row. |
| `features/status_attach.feature:30` | `!` (unseen marker) | row-scoped | No — unrelated glyph, not reason copy. |
| `features/status_probe.feature:55` | `sampled` | row-scoped | No — a verdict-source badge, not reason copy, never moved. |
| `features/permission_modes.feature:55` | `yolo (left/right cycles` | whole-frame, inside the create modal | No — create-modal copy, untouched by task 012. |
| `features/agent_session.feature:15,30`, `features/durable_identity.feature:40`, `features/permission_modes.feature:25,35`, `features/same_directory.feature:21` | `--continue`, `--dangerously`, `--approve`, a conversation id | audit-log argv, not screen text at all | No — out of scope for a screen-text sweep. |
| `internal/tui/resume_test.go:126`, `internal/tui/tui_test.go:80,103` | `starting elsewhere`, `awaiting signal` | Go view-model unit tests | Already re-aimed by task 012 itself (selects the row, checks the footer). |
| `internal/tui/profile_switch_test.go:207`, `internal/tui/badge_detail_test.go:66` | `yolo`, `Permission profile: safe` | Go view-model unit tests | No — unrelated copy (permission-profile badges), never moved. |

Only the six feature-file sites in the first two rows carry copy that task 012
relocated. Everything else in the sweep either polices a bare status word or
badge that always lived on the row, or copy from an unrelated subsystem
(create modal, permission profiles, audit log) that task 012 never touched.
No finding beyond these six sites required re-aiming.

### Why these six sites were already non-vacuous, and the injection proof

All six sites use the `deck client "A" screen does not contain "..."` step
(`clientScreenDoesNotContain`, `features/agent_steps_test.go:857`), which
checks `client.Frame(false)` — the *entire* rendered frame, footer included —
not a single row. Because the check was already whole-frame, relocating the
reason text from the row to the footer did not by itself make these
assertions vacuous: the footer was always inside the checked surface.

This was verified empirically rather than assumed, by deliberately injecting
each forbidden string into the footer and confirming the existing step still
fails:

1. **`starting elsewhere` (`launch_lease.feature`).** Temporarily removed the
   `m.resumeNote = ""` reset on a successful, non-conflicting resume in
   `internal/tui/tui.go`'s `sessionResumed` handler, so `resumeNote` would
   stick after a lease was cleared and the resume actually ran. Running only
   `@launch-lease` (`DECK_GODOG_TAGS="@launch-lease" go test -run TestFeatures
   ./features/...`) then failed exactly the expected step:
   `Scenario: a live in-TTL lease blocks a second launch and the row stays
   usable` → `And deck client "A" screen does not contain "starting
   elsewhere"` → `Error: ... screen unexpectedly contains "starting
   elsewhere"`. The other three `launch_lease` scenarios, which never take
   the "successful resume after a lease was live" path, were unaffected and
   still passed. The edit was reverted (`git diff --stat` clean afterwards).

2. **`awaiting signal` (`shell_liveness.feature`).** Temporarily made
   `Model.selectedRowReason()` unconditionally return `"starting · awaiting
   signal"` regardless of the selected session's actual status or agent.
   Running only `@shell-liveness` then failed both scenarios: `a live shell
   is promoted within one reconcile interval` on `screen does not contain
   "awaiting signal"`, and `a live unsignalled agent remains starting` on
   `screen still contains "starting - awaiting signal"` (it now saw the
   glyph-joined `·` form injected regardless of `DECK_ASCII`, which the ASCII
   dash-joined assertion no longer matched either — a second, incidental
   confirmation that the check is exact-substring and not merely "some status
   text present"). The edit was reverted (`git diff --stat` clean
   afterwards).

Both experiments demonstrate the step function inspects the whole frame, so
a regression that leaked the moved reason copy back onto an unselected
surface, held it open longer than it should, or otherwise let it linger would
be caught by the existing assertions exactly as before task 012. No feature
file needed rewording; the sweep's conclusion is that these six sites were
already correctly scoped and remain non-vacuous, and the proof above is the
evidence for that claim rather than an assumption.

### Suite status

`ci/run.sh go test -p=1 -count=1 ./...` is green after this task (no
production or feature-file changes were made — only the temporary,
fully-reverted injections described above, confirmed reverted via `git diff
--stat` showing no changes to `internal/tui/tui.go` before the final test
run).
