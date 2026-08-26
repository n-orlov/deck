# Phase 3f task 020 (R67 part 2): the two `Task 014` sections of `phase2b2-findings.md`, retitled after what they found

Parent commit: `300ee86`. Docs-only change: no Go file, feature file or golden frame is touched, so
no suite run gates it — the citation sweep below is the check that can go red, and being a pure text
scan it is deterministic and load-independent (no `/proc/loadavg` claim is made or needed).

All four logs cited below are committed beside this report in
`docs/reports/phase3f-020-r67-report-section-titles/`, with the same copies under
`/run/ralphd/artifacts/`.

## Why

`docs/reports/phase2b2-findings.md` had two sections that both opened `## Task 014 —`: `:1249` (the
SIGKILL after-scenario teardown hang) and `:1511` (requirement 19's live-apply fix defeating
requirement 21's env-over-file guarantee). Two unrelated investigations under one label, so a
citation "see the `Task 014` section" cannot be followed, and task ids reset per approach, so the
label does not even identify the work uniquely across the repo. This PRD itself cites the first of
the two.

## The two new titles

| line | before | after |
| --- | --- | --- |
| 1249 | `Task 014 — the SIGKILL scenario's after-scenario teardown hang is a harness-side coalesced-keystroke race, not a product defect` | `SIGKILL teardown hang — a harness-side coalesced-keystroke race in the PTY driver, not a product defect (2b-2 task 014)` |
| 1511 | `Task 014 — requirement 19's live-apply fix (task 006) defeated requirement 21's env-over-file guarantee (operator steer ...)` | `Requirement 19/21 correction — the live-apply fix (task 006) defeated requirement 21's env-over-file guarantee (operator steer `008-envoverride-applylive.md`, 22 Aug 2026 13:00 BST; 2b-2 task 014)` |

Both are one line each, so **the cited line numbers `:1249` and `:1511` still land on the same
sections** — `prds/phase3f-residuals-and-suite-determinism.md:88`, which cites
`docs/reports/phase2b2-findings.md:1249` by line, keeps resolving without an edit to a protected
file. Provenance is kept as a trailing lowercase `(2b-2 task 014)` rather than dropped: the subject
now leads the title, which is R67's whole point, while a reader can still find the task that filed
it.

`Requirement 19/21 correction` was not invented here — it is the title `docs/reports/phase2b2.md`
was **already** citing at two places (`:463`, `:592`), against a heading that did not exist. Those
two citations were dangling before this commit and resolve after it; both now also carry the
`:1511` line reference so they cannot silently drift again.

## Citations swept

`grep -rn 'Task 014' docs/ prds/`, before
(`docs/reports/phase3f-020-r67-report-section-titles/task020-r67-grep-task014-BEFORE.log`) and after
(`...-AFTER.log`). Every remaining hit is accounted for:

| hit | disposition |
| --- | --- |
| `docs/reports/phase2b2-findings.md:1249`, `:1511` | **retitled** (the two rows above) |
| `docs/reports/phase2b2.md:463`, `:592` | repointed: `"Requirement 19/21 correction"` + `(:1511)` |
| `docs/DELIVERY-LOG.md:393-401` | carried-items paragraph rewritten: R67 recorded as **done**, both halves, with the new titles and R67 part 1's shas |
| `docs/reports/phase3.md:101` | **not a citation of these sections.** It is Phase 3's own task 014 (the `E` event log) under a different numbering scheme, and the row says so itself |
| `docs/reports/phase2b2.md:894`, `:1111`, `:1115`, `:1153`, `:1185` | **not citations of a section title.** Narrative history — "this correction pass's task 014 fixed that SIGKILL-scenario teardown race" — which stays true and stays as written |
| `docs/reports/phase2b1-findings.md` (5 hits), `docs/reports/phase2b2-stability.log`, the `phase3*` suite logs | historical prose and captured tool output about *other* phases' task 014 / feature titles; captured logs are evidence and are never edited |
| `prds/phase3f-residuals-and-suite-determinism.md:480` | **FINDING, not edited.** `prds/` is protected for this job. The PRD quotes the pre-R67 duplicate title while describing the defect R67 removes, so after this commit that quotation is historical rather than a live pointer. Left exactly as written; the sweep prints it as a disclosed finding on every run |

## The sweep, and that it can go red

`docs/reports/phase3f-020-r67-report-section-titles/citation-sweep.py` (committed here) checks, over
all 96 tracked `*.md` under `docs/` and `prds/`, that every citation of a `phase2b2-findings.md`
section **by title** resolves to *exactly one* `##` heading (zero = dangling, more than one =
ambiguous), and that every citation **by line number** exists and is a heading where the prose calls
it one. Run from the repo root:

    $ python3 docs/reports/phase3f-020-r67-report-section-titles/citation-sweep.py
    docs/reports/phase2b2-findings.md: 23 '##' headings
    citations checked: 16 by title, 1 by line number, over 96 md files
    finding (disclosed, not a failure): docs/reports/phase2b2.md: 'Task 034' ambiguous, matches lines [770, 801, 849, 870, 1048] -- ...
    finding (disclosed, not a failure): docs/reports/phase2b2.md: 'Task 034' ambiguous, matches lines [770, 801, 849, 870, 1048] -- ...
    finding (disclosed, not a failure): prds/phase3f-residuals-and-suite-determinism.md: 'Task 014' dangling -- cites the pre-R67 title; prds/ is protected and must not be edited by this job
    OK: every section citation resolves to exactly one heading; no dangling line citation
    $ echo $?
    0

**Revert-and-reproduce.** The same script run in a throwaway `git worktree` at the parent commit
`300ee86` — i.e. with only the heading/citation fix absent — exits 1 on exactly this defect
(`docs/reports/phase3f-020-r67-report-section-titles/task020-r67-citation-sweep-BEFORE-RED.log`):

    PROBLEMS:
      - docs/DELIVERY-LOG.md: section citation 'Task 014' is ambiguous, matches lines [1249, 1511]
      - docs/reports/phase2b2.md: section citation 'Requirement 19/21 correction' is dangling
      - docs/reports/phase2b2.md: section citation 'Requirement 19/21 correction' is dangling
    exit=1

So the sweep is not vacuous: it fails without this commit and passes with it.

## Second finding, out of R67's scope

`phase2b2-findings.md` has **five** sections labelled `Task 034` (`:770`, `:801`, `:849`, `:870`,
`:1048`), and `phase2b2.md` cites "Task 034" by title twice — the same ambiguity the `Task 014` pair
had, one document over. R67 names only the `Task 014` pair, and retitling five sections plus their
citations is a change of its own size, so it is **not** done here: the sweep discloses it as a
finding on every run rather than letting it pass unseen. A later pass should retitle those five
after what they found (`docs/reports/phase2b2.md:917` already disambiguates one of them in prose by
quoting its subject, which is the pattern to follow).
