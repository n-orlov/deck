# Task 212 (I-21) — final hygiene and close-out

This is THE final close-out for the whole run (approach 2, tasks 201-220 plus the operator's four
steer-driven feature requirements). All of 201-211 and 213-220 are `completed` in `tasks.json`
before this task started (`git log` / `tasks.json` both checked — see "Dependency check" below).

## Dependency check

`tasks.json` at pickup time: every task 201-211 and 213-220 has `status: "completed"`. 212's own
`dependsOn: ["211"]` is satisfied, and its stricter clause ("may not be started while any task
201-211, 213-217 is pending, in-progress or failed") is also satisfied — none of those are
pending/in-progress/failed.

## 1. Git hygiene

```
$ git status --short
(empty)
$ git log origin/main..HEAD
(empty)
```

Both empty at HEAD `06379ee` (task 211's docs commit), confirmed at the start of this task and
re-confirmed after this task's own docs-only commit lands.

## 2. Build/vet/gofmt

```
$ ci/run.sh go build ./...
(no output, exit 0)
$ ci/run.sh go vet ./...
(no output, exit 0)
$ ci/run.sh sh -c "gofmt -l $(git ls-files '*.go')"
(no output, exit 0)
```

All three clean at HEAD `06379ee`.

## 3. The closing full-suite citation — GREEN, and at the true current code tip

Task 217's own declared "final code commit" (`029893a`) is **stale**: two further code commits
landed on top of it as part of task 210's work — `3a26ccb` (tmux: drain pane chatter before
asserting pipe displacement/disable EOF) and `f715a56` (features: fix delete-undo/restore DB-read
race in `kill_delete_undo.feature`). Task 217's attempt-7 green run
(`docs/reports/phase3d-217-final-sweep/full-suite-run-attempt7.log`) does not exercise either of
those two commits' changes, so it cannot honestly serve as *this* task's closing citation — the
run must be green **at the commit that is actually current**.

The current code tip is **`f715a56`** (every commit after it — `ea44a30`, `d59ba28`, `06379ee` —
is docs-only; confirmed via `git diff --name-only f715a56..HEAD`, which touches only files under
`docs/reports/` plus nothing else). Task 210 already collected the strongest possible green
full-suite evidence at that exact tip: **`ci/stability.sh 10`**, i.e. **ten independent, complete,
clean-state runs** of `ci/run.sh go test -p=1 -count=1 ./...`, each one individually exit-0 with
every one of the 14 testable packages `ok` and zero `FAIL` lines. Evidence:
`docs/reports/phase3d-210-stability-10of10/{README.md,summary.log,run-1.log..run-10.log}`
(docs commit `d59ba28`).

**This task's closing citation is that same evidence** — specifically
`docs/reports/phase3d-210-stability-10of10/run-1.log` as the single canonical exemplar (any of the
ten would do; all ten are identical in outcome: exit 0, every package `ok`), with the full set of
ten kept as the complete record. This supersedes task 217's `029893a` citation in
`docs/reports/phase3.md` row 47 (updated below) precisely because it is both green AND current —
10 runs, not 1, and 2 further code commits ahead of `029893a`.

No fresh whole-suite run was started by this task: task 210's own measurement already is a green
full-suite run at the current tip, run within this task's own one-whole-suite-run-per-iteration
budget window (i.e. re-running it here would just repeat work already on record with no new
information, and would spend this iteration's one whole-suite-run allowance for nothing).

## 4. Protected-path check — three formulations, reported in order, sha-discriminated

Full raw output: `docs/reports/phase3d-212-closeout/protected-path-check.log`.

**Formulation 1 (task 077 / composite-prd.md:744,378-379): "empty diff since `9cda5a8`".**
Broken by the operator's own legitimate `395babf` SPEC push partway through this run — after that
push the diff is no longer empty through no fault of this run, so a literal reading of this
formulation would fail close-out for a change this run did not make. Superseded by steer 018 §0.

**Formulation 2 (steer 018 §0): "by authorship"** —
`git log 9cda5a8..HEAD --format='%H %an <%ae>' -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md`,
requiring every listed commit be the operator's and none the run's own committer identity.
**Broken, and provably so**: this run's own committer identity and the operator's pushed-commit
identity are the byte-identical string `Nik <nikolaiorl@gmail.com>` on *both* `%an/%ae` and
`%cn/%ce` (both come from the same mounted repo's `.git/config`) — confirmed directly above (see
`protected-path-check.log`'s "byte-identical identity check" section: `395babf`'s author/committer
line and this run's own most recent commit's author/committer line are identical). A protected-path
edit made by this run would pass formulation 2's check. Superseded by steer 019 §1.

**Formulation 3 (steer 019 §1, refined by steer 022) — sha allow-list, discriminated by sha only,
never by authorship/committer identity, never by subject-line/path heuristics:**

```
git log 9cda5a8..HEAD --format='%h %s' -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
```

Result at this task's HEAD (`06379ee`):

```
395babf spec: deck is the primary view of a session — preview fits on navigation, drag-to-copy, modified keys forward, yolo de-gated
```

**Exactly one line, and it is `395babf`.** `395babf` is positively identified as the operator's own
commit by an external, unforgeable reference: **steer 019 §1 announced it in advance**, quoting its
exact subject line ("spec: deck is the primary view of a session — preview fits on navigation,
drag-to-copy, modified keys forward, yolo de-gated"), before this check was ever run in this task —
that announcement, not anything inspectable in the repo itself, is the source of the identification
(steer 022 restates and endorses this: *"`395babf` is a positively identified operator commit, and
its identification does not rest on anything in the repo: it was announced to you in steer 019 §1,
in advance, with its exact subject line quoted"*).

**No other sha appears in the range.** Per steer 022's explicit instruction, this is checked as a
hard pass/fail, not a classification exercise: any sha other than `395babf` in that output would
fail this task outright, named and unclassified, asking the operator to confirm it — that did not
happen here.

Per path, independently (full detail in `protected-path-check.log`):
- `SPEC.md`: exactly one commit, `395babf` (the recognized operator commit above).
- `prds/`: **zero** commits in the range.
- `ci/Dockerfile`: **zero** commits in the range.
- `ci/SPIKE.md`: **zero** commits in the range.

Protected-path check: **PASS.**

## 5. I-19 parity assertion — re-run fresh, green

Help overlay keymap parity, both directions (`internal/tui/help_keymap_parity_test.go`):

```
$ ci/run.sh go test -count=1 -run 'TestHelpOverlayKeymapMatchesBoundKeys|TestFooterKeyLegendNamesOnlyBoundKeys' ./internal/tui/ -v
=== RUN   TestHelpOverlayKeymapMatchesBoundKeys
--- PASS: TestHelpOverlayKeymapMatchesBoundKeys (0.00s)
=== RUN   TestFooterKeyLegendNamesOnlyBoundKeys
--- PASS: TestFooterKeyLegendNamesOnlyBoundKeys (0.00s)
PASS
ok  	github.com/n-orlov/deck/internal/tui	0.011s
```

Full log: `docs/reports/phase3d-212-closeout/i19-parity-rerun.log`. Both directions
(help-lists-an-unbound-key and bound-key-undocumented) covered by
`TestHelpOverlayKeymapMatchesBoundKeys`; the footer's own separate legend gets
`TestFooterKeyLegendNamesOnlyBoundKeys`. Re-run at HEAD `06379ee`, at the same code tip (`f715a56`)
as every other citation in this report.

## 6. Result

All of task 212's success criteria are met:
- `git status --short` / `git log origin/main..HEAD` both empty.
- `go build`/`go vet`/`gofmt -l` clean.
- Closing full-suite citation is GREEN (exit 0, every package `ok`) and at the true current code
  tip (`f715a56`), not a stale or red run — `docs/reports/phase3d-210-stability-10of10/run-1.log`
  (and its 9 siblings).
- Protected-path check done with all three formulations named and the reason each earlier one was
  broken stated; the sha-allow-list formulation passes with exactly the one recognized commit.
- I-19 parity re-run and green, log cited.

**This run's own work (approach 2: tasks 201-220 plus the operator's four steer-driven feature
requirements) is complete.**
