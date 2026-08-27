# Phase 3f: R74/R75 fixing shas posted to GitHub issue #11 (task 037)

Issue [#11](https://github.com/n-orlov/deck/issues/11) — "Restart leaves a row 'stopped'
with a live pane" — is the field report that operator steer 002 turned into **two**
authorised requirements, **R74** (a late hook from a superseded launch generation is
dropped) and **R75** (a completed launch releases its launch lease). Both are implemented,
so the issue received one comment naming the fixing shas, the regression tests, the
revert-and-reproduce quotes and the evidence paths. Posted through the GitHub REST API
(`POST /repos/n-orlov/deck/issues/11/comments`) with the token in `~/.git-credentials`.

**The issue was not closed, and this record does not authorise closing it.** #11 was
`open` before and after; state was re-read after posting (table below). The comment's own
last line says so, and it contains no GitHub closing keyword followed by an issue number —
scanned, `CLEAN`.

## The comment, and the API response that records it

| field | value (from the `201` response body) |
|---|---|
| issue | #11, `n-orlov/deck` |
| POST status | **201** |
| comment id | **5433783057** |
| author | **`n-orlov`** |
| created_at | **2026-08-27T02:57:17Z** (`updated_at` identical — never edited) |
| permalink | <https://github.com/n-orlov/deck/issues/11#issuecomment-5433783057> |
| body length | 7172 characters / 7210 bytes UTF-8, byte-identical to the artifact copy |

What the comment names as the fix:

| requirement | fixing sha(s) | mechanism |
|---|---|---|
| **R74** leg 1 | `a0d4887` | `AcquireLaunchLease` mints a **random** per-launch generation token, stores it as the second half of `launch_lease_owner` (`pid@boot_id#generation`, no new column) and exports it beside `DECK_SESSION_ID` |
| **R74** leg 2 | `196e6f4` | `internal/hookrecv` drops a hook write whose `DECK_LAUNCH_GENERATION` is not the row's current one, for **every** event name; a tokenless hook is deliberately still allowed |
| **R75** | `0a5034d` | `store.ReleaseLaunchLease` zeroes `launch_lease_until` only, CASed on the acquiring owner; `Resume` releases by `defer`. Acquisition unchanged, §9.3 guard not weakened, owner column kept because it *is* R74's discriminator |

The comment also carries the two deliverable runs at the final code sha `0a5034d` — whole
suite green and ten-run stability **10/10**, the latter including `lease_race.feature`, the
watch item R75 named in advance — and states what was deliberately **not** done:
`reconcile.go`'s "terminal row + live pane" invariant detector (the issue's suggestion 3)
remains open as a follow-up finding, the two protected `store.go` guards are untouched
(checked in `196e6f4`'s diff, where they do not appear), and R75's best-effort release
fallback is exercised by no test.

## Evidence paths the comment cites, and why each exists

Every path the comment names is repo-relative and resolves at `f8174f7` (checked with `ls`
this iteration), and every sha it names resolves (`git cat-file -e`):

| cited path | holds |
|---|---|
| `docs/reports/phase3f-029-r74-launch-generation/` | R74 leg 1: `README.md`, `green-store-agent-service.log`, `hookrecv.log`, and the three reverts (`revert-a-no-generation-persisted.log`, `revert-b-identity-parse-keeps-suffix.log`, `revert-c-no-env-injection.log`) |
| `docs/reports/phase3f-030-r74-superseded-hooks/` | R74 leg 2: `README.md`, `green-service-hookrecv-store.log`, `revert-a-no-drop-at-all.log`, `revert-a-service-ordering-verbose.log` (the measured before/after-lease ordering control), `revert-b-sessionend-only.log`, `revert-c-tokenless-hook-allowed.log`, `features-hook-env-fixture.log`, and the published failing `whole-suite.log` |
| `docs/reports/phase3f-031-r75-launch-lease-release/` | R75: `README.md`, `green-store-service.log`, `features-lease.log`, `revert-a-no-release.log`, `revert-b-guard-deleted.log`, `revert-c-release-blanks-the-owner.log` |
| `docs/reports/phase3f-032-fullsuite/` | whole suite green at `0a5034d` (`go-test-p1-count1-all.log`) |
| `docs/reports/phase3f-033-stability10/` | ten-run stability 10/10 at `0a5034d` (`run-1.log`…`run-10.log`, `stability-summary.log`, `loadavg-trace.log`) |
| `docs/reports/phase3f.md` | the R74 and R75 sections the comment paraphrases |
| `docs/reports/phase3f-findings.md` | the follow-up finding recording the reconcile detector as out of scope |

Test-file names quoted in the comment — `internal/service/superseded_launch_hook_test.go`,
`internal/store/launch_generation_test.go`, `internal/service/launch_generation_test.go`,
`internal/store/lease_release_test.go`, `internal/service/lease_release_test.go` — all exist
on `main`. No claim in the comment is new: each is a restatement of `docs/reports/phase3f.md`
or of the three task reports above.

## Issues #5–#10 were not re-commented, and #12 was not touched

Verification is a second code path from the one that posted: after the POST, each issue and
each issue's `/comments` collection was fetched with `curl` and the newest comment compared
against the ids task 026 recorded in
[`phase3f-026-issue-notifications.md`](phase3f-026-issue-notifications.md).

| issue | state | comments | newest comment id | newest at | matches task 026's record |
|---|---|---|---|---|---|
| #5 | open | 2 | 5432079186 | 2026-08-26T22:59:14Z | yes — unchanged |
| #6 | open | 1 | 5432079373 | 2026-08-26T22:59:15Z | yes — unchanged |
| #7 | open | 1 | 5432079694 | 2026-08-26T22:59:18Z | yes — unchanged |
| #8 | open | 3 | 5432079788 | 2026-08-26T22:59:19Z | yes — unchanged |
| #9 | open | 2 | 5432079863 | 2026-08-26T22:59:19Z | yes — unchanged |
| #10 | open | 1 | 5432079953 | 2026-08-26T22:59:20Z | yes — unchanged |
| #11 | open | 2 | **5433783057** | **2026-08-27T02:57:17Z** | new — this task's comment, the newest on the issue |
| #12 | open | 0 | — | — | untouched |

So the only write this task made to GitHub is comment `5433783057` on #11. #12 (the 30 s
launch-lease finding split out of #11) is the issue whose *mechanism* R75 fixes, but posting
there was outside this task, so it has no comments and its state was only read.

## No closing keyword, and no issue closed

The body was scanned for a closing verb followed by an issue number
(`close`/`closes`/`closed`/`fix`/`fixes`/`fixed`/`resolve`/`resolves`/`resolved` immediately
before `#N` or a bare number, case-insensitive) before posting: `CLEAN`, no match. The
issue is referenced as a bare `#11` throughout, matching the run's standing rule that every
R74/R75 commit and comment names issue #11 as a bare reference. #11 read `state: open` both
before (1 comment) and after (2 comments) the POST.

## Artifacts

Under the run's artifacts directory, `task037-issue-comments/`: the comment body exactly as
posted (`issue-11-r74-r75.md`), the API POST log with the `201` and the id/author/timestamp
it returned (`api-post-responses.log`), and the post-hoc `curl` verification of #5–#12
(`curl-verification.log`). The token was never printed and never passed as a command
argument: it was parsed out of `~/.git-credentials` into a `curl` config file created with
`umask 077` (mode 600) holding the `Authorization` header, used via `curl -K`, and deleted
afterwards (verified absent).
