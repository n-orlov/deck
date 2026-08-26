# Phase 3f — task 021: green whole-suite run at the final code commit

**Commit under test**: `e47cb35` (`docs: title the two duplicate report
sections after what they found (task 020)`) — the phase's tip at the time of
the run, working tree clean (`git status --short` empty, only this report
directory untracked afterwards).

`e47cb35` is a docs-only commit, as is its parent `300ee86`; the last commit
that touches compiled code is `b848d28` (task 019). The tree under test is
therefore byte-identical to `b848d28`'s in every source file, which is checked
rather than asserted:

```
$ git diff --stat b848d28 e47cb35 -- '*.go' '*.feature' '*.toml' '*.sh' go.mod go.sum
$                     # empty: no code difference
```

**Command**: `ci/run.sh go test -p=1 -count=1 ./...` (the sibling toolchain
container; no tag or path filter, no `-run`, nothing excluded).

**Log**: [`go-test-p1-count1-all.log`](./go-test-p1-count1-all.log) — raw
stdout with `EXIT=0` appended. Also copied to the run artifacts as
`task021-fullsuite-green.log`.

**Result**: exit 0; all 17 package lines are `ok` or `[no test files]` —
14 × `ok` and 3 × `[no test files]` (`internal/notify`, `internal/search`,
`internal/unit`). No
`FAIL`, no `---` failure block, no build error in the log:

```
$ awk 'length($0)<400 && $0 !~ /^(ok|\?)/' go-test-p1-count1-all.log | grep -v EXIT=0
$                     # empty
```

**Tool versions** (in the same image the suite ran in):

```
$ ci/run.sh go version
go version go1.25.13 linux/amd64
$ ci/run.sh tmux -V
tmux 3.5a
```

**Wall clock**: 360 s (6 m 00 s), start `2026-08-26T19:04:45Z`
(unix `1787858685`). `features` alone accounts for 308.184 s of it, in line
with the 282–313 s this phase has measured for that package.

**1-min loadavg** (`cat /proc/loadavg`, 28-core host):

| moment | /proc/loadavg |
| --- | --- |
| start of run | `2.69 2.60 2.82 2/4123 22874` |
| end of run | `3.82 4.29 3.60 20/4229 23037` |

## This is one green run, and one green run is not the stability claim

Two known intermittent reds live in this suite. Neither fired here, and this
report does not treat their absence as evidence that they are gone. **No run
was discarded and none was repeated**: the run above is the first and only
whole-suite invocation made at `e47cb35`, so there is no streak-hunting behind
the green.

1. **`preview.feature:134`** (`a fit is skipped below the …`) failed 1 of the
   4 whole-suite runs recorded earlier in the phase — `received 1 SIGWINCH
   signals, want exactly 0` at `preview.feature:147`
   (`artifacts/task017-r65-full-suite-run3-RED-trimmed.log`), and the same
   scenario failed the same way before R65 existed
   (`artifacts/task015-r63-features-exacttags-flake.log`). It is root-caused in
   `docs/reports/phase3f-017-r65-settled-sigwinch-count.md` (section
   "FINDING — `preview.feature:134` has a pre-resize fit race that R65 exposes
   rather than causes"): `beacon` is selected on creation at 100x30, so a
   passive fit is licensed *before* the resize to 100x9, and whether it issues
   a real `resize-window` depends on whether the pane exists when the
   coalesced tick fires. Fixing it means restructuring the scenario's prefix so
   no fit is licensed at the larger size; re-baselining its counts is
   forbidden. **Still open** — it needs its own task and is expected to show up
   in the `ci/stability.sh 10` deliverable (task 022), whose published rate is
   the honest stability number.
2. **`TestGoldenMinimumFrame`** ("frame kept changing") recurred at task 016
   with a new signature — a footer trailing column rather than preview geometry
   (`artifacts/task016-goldenframe-count10*.log`). It too passed here
   (`internal/tui  0.771s`).

Read together: this task's criterion is a green *whole-suite* run at the final
code commit, and it is met above. The rate at which that green repeats is task
022's subject, not this one's, and the two findings above are carried into it
and into `docs/reports/phase3f-findings.md`.
