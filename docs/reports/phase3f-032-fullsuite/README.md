# Phase 3f — task 032: green whole-suite run at the final code commit

## 1. What was run, where, and at which sha

**Commit under test (the sha):** `0a5034d8bf7195541ca8b252956369db442b0024`
(`store: release a concluded launch's lease so its own row is not "starting
elsewhere" (task 031)`) — the phase's tip after tasks 028, 029, 030 and 031, and
the last commit in the phase that touches compiled code:

```
$ git log --oneline -4 0a5034d
0a5034d store: release a concluded launch's lease so its own row is not "starting elsewhere" (task 031)
196e6f4 hookrecv: drop a hook write from a superseded launch generation (task 030)
a0d4887 store: mint a per-launch generation for each launch lease (task 029)
2b39124 features: stop preview.feature's floor scenario racing its own shrink (task 028)
```

**Command:** `ci/run.sh go test -p=1 -count=1 ./...` — the whole module in the
sibling toolchain container, no `-run`, no tag filter, no path filter, nothing
excluded or skipped. `-p=1` serialises packages (the `features` and
`internal/tmux` packages both drive real tmux servers) and `-count=1` defeats
the test cache, so every test in the module actually executed in this run.

**Working tree at the moment of the run:** clean — `git status --short` printed
nothing before the run was started (this report's directory is the only
untracked path afterwards). So the tree the suite compiled is exactly
`0a5034d`'s tree, not a dirty variant of it.

**Log:** [`go-test-p1-count1-all.log`](./go-test-p1-count1-all.log) — the raw
stdout+stderr of the command with `EXIT=$?` appended by the runner. Copied
unchanged to the run artifacts as `task032-fullsuite-green.log`.

## 2. Result

**Exit status 0.** All 17 package lines read `ok` or `[no test files]` —
14 × `ok`, 3 × `[no test files]` (`internal/notify`, `internal/search`,
`internal/unit`, none of which has ever had tests):

```
ok  	github.com/n-orlov/deck/cmd/deck	5.264s
ok  	github.com/n-orlov/deck/cmd/fake-claude	0.795s
ok  	github.com/n-orlov/deck/cmd/fake-pi	0.776s
ok  	github.com/n-orlov/deck/features	307.182s
ok  	github.com/n-orlov/deck/internal/agent	0.005s
ok  	github.com/n-orlov/deck/internal/audit	0.018s
ok  	github.com/n-orlov/deck/internal/config	0.025s
ok  	github.com/n-orlov/deck/internal/hookrecv	4.436s
ok  	github.com/n-orlov/deck/internal/interactive	11.071s
?   	github.com/n-orlov/deck/internal/notify	[no test files]
?   	github.com/n-orlov/deck/internal/search	[no test files]
ok  	github.com/n-orlov/deck/internal/service	3.911s
ok  	github.com/n-orlov/deck/internal/store	1.911s
ok  	github.com/n-orlov/deck/internal/theme	0.005s
ok  	github.com/n-orlov/deck/internal/tmux	19.191s
ok  	github.com/n-orlov/deck/internal/tui	0.664s
?   	github.com/n-orlov/deck/internal/unit	[no test files]
EXIT=0
```

The log is 18 lines and holds nothing else — no `FAIL`, no `--- FAIL` block, no
build or vet error, no `panic`, no godog failure summary. Checked rather than
eyeballed, by asking for every line that is neither a package line nor the exit
marker:

```
$ awk 'length($0)<400 && $0 !~ /^(ok|\?)/' go-test-p1-count1-all.log | grep -v '^EXIT=0$'
$                     # empty
```

This was the **first and only** whole-suite invocation made at `0a5034d`. No run
was discarded, none was repeated, and no scenario, tag or package was excluded
to obtain it.

## 3. Toolchain versions

Queried in the same image the suite ran in (`deck-ci:local`, via the same
`ci/run.sh` wrapper), immediately after the run:

```
$ ci/run.sh go version
go version go1.25.13 linux/amd64
$ ci/run.sh tmux -V
tmux 3.5a
$ ci/run.sh go env GOVERSION CGO_ENABLED
go1.25.13
1
```

Host kernel/arch as seen by the sibling: `linux/amd64`, 28 logical CPUs
(`nproc` = 28 in this container, which shares the host's CPU view).

## 4. Timing and host load

| moment | value |
| --- | --- |
| start (UTC) | `2026-08-27T00:55:08Z` (unix `1787792108`) |
| end (unix) | `1787792468` |
| wall clock | **360 s** (6 m 00 s) |
| `/proc/loadavg` at start | `2.67 3.25 3.15 6/4228 84586` |
| `/proc/loadavg` at end | `3.12 3.15 3.13 14/4161 85372` |

Both loadavg samples were taken in this container immediately before the run was
launched and immediately after it exited, and are recorded because host load is a
known confound for this suite's timing. At ~3 of 28 cores the host was lightly
loaded throughout, and the 360 s total matches the phase's established profile:
`features` alone is 307.182 s of it, next to the 308.184 s task 021 measured for
that package at `e47cb35`, the 305.551 s of task 030's red run and the 300.143 s
of phase 3e task 407's run
(`docs/reports/phase3f-021-fullsuite/go-test-p1-count1-all.log`,
`docs/reports/phase3f-030-r74-superseded-hooks/whole-suite.log`,
`docs/reports/phase3e-407-whole-suite-at-75861e0/go-test-p1-count1-all.log`).

## 5. The measured tree is the shipped code tree

The point of running at a named sha is that the shipped code is the code that was
measured. At the time this report was written, `HEAD` was still `0a5034d` itself,
so the diff between the measured sha and the tree being shipped is empty:

```
$ git rev-parse HEAD
0a5034d8bf7195541ca8b252956369db442b0024
$ git diff --name-only 0a5034d HEAD
$                     # empty: HEAD *is* 0a5034d
$ git status --short
?? docs/reports/phase3f-032-fullsuite/
```

The only uncommitted path was this report directory. The commit that lands this
report therefore adds paths under `docs/` and nothing else, which is what the
commit itself shows (`git show --stat`); every later commit in this phase
(tasks 033–038) is likewise a report/docs commit, so
`git diff --name-only 0a5034d HEAD` continues to touch nothing outside `docs/`.
If any later task does change compiled code, this run stops being the final
code-commit measurement and must be redone at the new tip — that is the standing
condition on this report, and tasks 033–038 are all documentation tasks by
design.

## 6. One green run is not the stability claim

This report claims exactly one thing: at `0a5034d`, one full `-p=1 -count=1`
run of every package passed, with the log to prove it. It does **not** claim the
suite is flake-free. The repeated-run measurement is task 033
(`ci/stability.sh 10` at this same sha), and that report — not this one — carries
the stability rate.

Two things this phase knows about are worth naming so their absence here is not
mistaken for evidence:

- Finding F1's `preview.feature` passive-fit race, fixed in `2b39124`
  (`docs/reports/phase3f-028-f1-passive-fit-floor/`). It did not fire here, but a
  single green run is exactly what F1 looked like 9 times out of 10.
- `features/lease_race.feature`'s "at least one racer shows starting elsewhere"
  assertion, which post-R75 depends on a loser observing the winner's launch
  while it is genuinely in flight (`docs/reports/phase3f-031-r75-launch-lease-release/README.md`
  §6). It passed here as part of the green `features` package; if it ever fails,
  that mechanism is the first suspect rather than host load.

No run was hidden, repeated or discarded to produce this page.
