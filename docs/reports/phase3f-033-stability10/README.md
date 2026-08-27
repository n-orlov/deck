# Phase 3f — task 033: `ci/stability.sh 10` at the final code commit, real rate published

**Rate: 10/10. Script exit status: 0.** Ten runs commissioned, ten run, none
re-run; the verdict line and exit status are quoted verbatim in §2 and every run's
start `/proc/loadavg` is in §3.

## 1. What was run, where, and at which sha

| | |
|---|---|
| command | `ci/stability.sh 10` — nothing else: no tag selector, no path filter, no `DECK_*` env override, no `-run` |
| per-run command inside the script | `ci/run.sh go test -p=1 -count=1 ./...`, one throwaway `--rm` sibling container per run |
| checked-out commit | `9ce65be` (`docs: publish the green whole-suite run at the final code commit (task 032)`) |
| code sha exercised | **`0a5034d`** (`store: release a concluded launch's lease so its own row is not "starting elsewhere" (task 031)`) — the same code sha task 032's whole-suite run measured |
| worktree | clean before the run and clean after it (`git status --short` empty both times) |
| toolchain | go1.25.13 linux/amd64, tmux 3.5a (inside the sibling image `deck-ci:local`) |
| start | `1787793327` unix = `2026-08-27T01:15:27Z`, `/proc/loadavg` `2.77 3.24 3.14 4/4186 87693` |

`9ce65be` is a documentation-only commit on top of `0a5034d`, so the tree these ten
runs compiled and exercised is `0a5034d`'s:

```
$ git rev-parse HEAD
9ce65be10568d958fac130a22f2c8f5465120231
$ git diff --stat 0a5034d HEAD -- '*.go' '*.feature' '*.toml' '*.sh' go.mod go.sum
                        # empty: no Go, feature, theme, script or module change
$ git diff --name-only 0a5034d HEAD
docs/DELIVERY-LOG.md
docs/reports/phase3f-032-fullsuite/README.md
```

This is the requirement's "same code sha as task 032": task 032's report
([../phase3f-032-fullsuite/README.md](../phase3f-032-fullsuite/README.md)) records
its single `go test -p=1 -count=1 ./...` run at that same `0a5034d` tree, from
commit `9ce65be`.

One unrelated sibling container ran beside run 1 for two seconds:
`ci/run.sh sh -c 'go version; tmux -V'`, to capture the toolchain versions in the
row above. It compiles and runs nothing of the suite. It is disclosed rather than
omitted because it is visible in this report's load trace, and run 1's own outcome
is reported below whatever it is — nothing was re-run to remove it.

## 2. The script's own verdict line and exit status, verbatim

The last two lines the script printed on stdout, quoted exactly as they appeared
(from [run-markers-timestamped.log](run-markers-timestamped.log), the same text as
the tail of [stability-summary.log](stability-summary.log)):

```
full per-run logs and combined summary log kept in: /tmp/deck-stability.ItoG43
10/10 passed
```

And the script's exit status, captured by the wrapper immediately after the script
returned and written to a file before anything else ran:

```
$ cat /tmp/stab033/exit.txt
EXIT=0
```

**Rate: 10/10. Script exit status: 0.** `ci/stability.sh` exits `1` whenever
`fail > 0`, so exit `0` and `10/10 passed` are two independent expressions of the
same result: no run failed.

End of the run: `1787796931` unix = `2026-08-27T02:15:31Z`; total wall clock
`1787796931 - 1787793327` = **3604 s = 60m04s** for the ten runs.

## 3. Per-run table, with each run's start `/proc/loadavg`

Every row's `start /proc/loadavg` is the host's `/proc/loadavg` read in the same
second that run's `=== RUN n ===` marker appeared — all ten are present, none is
interpolated. `features` is the Godog package, the long pole of every run.

| run | verdict (script's own line) | start unix | end unix | wall | `features` pkg | start `/proc/loadavg` (1/5/15 min) |
|---|---|---|---|---|---|---|
| 1 | `=== RUN 1: PASS (exit 0) ===` | 1787793328 | 1787793691 | 363 s | 311.283 s | `2.77 3.24 3.14` |
| 2 | `=== RUN 2: PASS (exit 0) ===` | 1787793691 | 1787794048 | 357 s | 305.848 s | `3.32 3.56 3.30` |
| 3 | `=== RUN 3: PASS (exit 0) ===` | 1787794048 | 1787794403 | 355 s | 304.419 s | `3.19 3.69 3.50` |
| 4 | `=== RUN 4: PASS (exit 0) ===` | 1787794403 | 1787794764 | 361 s | 309.469 s | `2.55 3.51 3.55` |
| 5 | `=== RUN 5: PASS (exit 0) ===` | 1787794764 | 1787795129 | 365 s | 312.046 s | `2.81 3.07 3.34` |
| 6 | `=== RUN 6: PASS (exit 0) ===` | 1787795129 | 1787795488 | 359 s | 307.955 s | `4.56 4.17 3.75` |
| 7 | `=== RUN 7: PASS (exit 0) ===` | 1787795488 | 1787795849 | 361 s | 309.177 s | `2.39 3.09 3.41` |
| 8 | `=== RUN 8: PASS (exit 0) ===` | 1787795849 | 1787796212 | 363 s | 312.106 s | `2.48 2.99 3.29` |
| 9 | `=== RUN 9: PASS (exit 0) ===` | 1787796212 | 1787796573 | 361 s | 308.247 s | `5.38 3.89 3.49` |
| 10 | `=== RUN 10: PASS (exit 0) ===` | 1787796573 | 1787796932 | 359 s | 307.075 s | `5.04 4.12 3.68` |

Every run produced the **same 17 package lines — 14 `ok`, 3 `[no test files]`** (140
`ok` and 30 `?` lines across the ten logs, i.e. 14 and 3 per run), so no run silently
exercised a smaller package set than another. Per-run logs are committed here as
[run-1.log](run-1.log) … [run-10.log](run-10.log), untrimmed — 894 bytes each, since
a fully green `go test` prints one line per package and nothing else.

### Host load over the hour

`/proc/loadavg` was also sampled every 5 s for the whole run into
[loadavg-trace.log](loadavg-trace.log) (721 samples, each line
`<unix> <1min> <5min> <15min> <procs> <lastpid> | <last RUN marker seen>`):

| | 1-minute load |
|---|---|
| minimum sample | `2.04` |
| mean of 721 samples | `3.82` |
| maximum sample | `11.58` |

The host was **not** idle: it carried a background load of ~2–4 throughout, with
spikes to 11.58, and the two most-loaded run starts (runs 9 and 10, `5.38` and
`5.04`) both passed in 361 s and 359 s — within 3 s of the quietest run's 359 s.
So this 10/10 is not a quiet-host artefact, and per-run wall clock is insensitive to
that load band (spread 355–365 s, 2.8 %).

Both companion samplers were read-only (`/proc/loadavg` and the script's own stdout
file); neither touched the suite, the script, or its exit status.

## 4. Failures: root cause per failing run, and whose mechanism it is

**There were no failing runs.** All ten runs' verdict lines are `PASS (exit 0)`
(§3), the script's own summary is `10/10 passed`, and its exit status is `0` — so
there is no failure to root-cause, and this section is empty of failures by
measurement, not by omission. Concretely, the evidence that would carry a failure is
absent from all ten logs:

```
$ grep -lE '^(FAIL|--- FAIL)' docs/reports/phase3f-033-stability10/run-[0-9]*.log; echo "exit=$?"
exit=1                                   # no output, no match: no run has a FAIL line
$ grep -c '^ok' docs/reports/phase3f-033-stability10/run-[0-9]*.log \
    | sed 's|docs/reports/phase3f-033-stability10/||' | tr '\n' ' '
run-1.log:14 run-10.log:14 run-2.log:14 run-3.log:14 run-4.log:14 run-5.log:14 \
run-6.log:14 run-7.log:14 run-8.log:14 run-9.log:14
```

The two mechanisms this section exists to interrogate, and where they stand:

- **The mechanism that cost the previous 9/10 (finding F1) did not recur.** Task 022's
  single failure was `features/preview.feature`'s fit-floor scenario racing its own
  shrink — a SIGWINCH-count assertion — root-caused and fixed by task 028
  (`2b39124`, report [../phase3f-028-f1-passive-fit-floor/README.md](../phase3f-028-f1-passive-fit-floor/README.md),
  with a deterministic red-on-revert reproduction there). Ten consecutive green
  `features` packages here (304–312 s each) are consistent with that fix and contain
  no instance of it. That is an *absence* claim from ten samples, so it is stated as
  such: it is not proof the class is impossible, it is the required ten-run evidence
  that it did not fire once at `0a5034d`.
- **R75's watch item did not fire either.** `features/lease_race.feature` asserts that
  at least one racer reports "starting elsewhere"; after task 031 (`0a5034d`) a loser
  only observes that while the winner's launch is genuinely in flight, which is the
  narrowest window this assertion has ever had. It is inside the `features` package
  in every run above, and every one of those was `ok`, so the window held ten times
  out of ten. Per the notes' standing watch item, had a run failed there, **that
  mechanism would have been R75's own** — R75 is the requirement that narrowed the
  window — and it would have been written up here as a failed requirement, not as a
  host-load note.

Because no run failed, no run needed re-running, and none was re-run (§5).

## 5. Nothing was skipped, tagged out, or re-run for a streak

- **Ten runs were commissioned and ten were run.** The count argument was `10`; the
  script's loop is `while [ "$i" -le "$runs" ]` and it prints one
  `=== RUN n ===` / `=== RUN n: PASS|FAIL ===` pair per iteration. The marker log
  ([run-markers-timestamped.log](run-markers-timestamped.log)) contains exactly ten
  of each, numbered 1–10, with no gap and no repeat — so no run was discarded and
  none was replayed.
- **This is the only ten-run measurement taken this iteration**, and it is the one
  published. No earlier ten-run attempt at `0a5034d` was started and abandoned; if a
  second had been taken it would appear here beside this one, per the phase's
  "publish every measurement taken" rule. Task 022's earlier 9/10 at the *previous*
  approach's code sha (`e47cb35`) also remains published, unedited, at
  [../phase3f-022-stability10/README.md](../phase3f-022-stability10/README.md).
- **No scenario was deleted, skipped, `@flaky`-tagged or tag-excluded** to reach this
  rate. `defaultTags` is still `"~@real-agents && ~@nightly"` at `0a5034d`, unchanged
  from before the phase; the script passes no tag override, so every non-nightly,
  non-real-agent scenario ran in every one of the ten runs.
- **No retry loop, no `sleep`, no widened timeout** was added for this measurement:
  the tree is byte-identical to task 032's, shown in §1.
- **The pass/fail label is `go test`'s own exit status**, captured by the script
  immediately after the command that produced it and never through a pipe
  (`ci/stability.sh`'s central anti-`tee` invariant), so a red run cannot be
  mislabelled PASS.

## 6. Logs kept with this report

| path | what it is |
|---|---|
| [run-1.log](run-1.log) … [run-10.log](run-10.log) | each run's own `go test -p=1 -count=1 ./...` output, copied verbatim from the script's `/tmp/deck-stability.ItoG43/run-N.log`. Untrimmed: 894 bytes each. |
| [stability-summary.log](stability-summary.log) | the script's own combined `summary.log`: the ten `=== RUN n ===` / `=== RUN n: PASS ===` marker pairs interleaved with each run's package lines, ending in `10/10 passed`. |
| [run-markers-timestamped.log](run-markers-timestamped.log) | every stdout line the script printed, each stamped with the unix second it appeared, and each `=== RUN` marker followed by a `LOADAVG-AT-MARKER` line — this is the source of §3's start times and start loads. |
| [loadavg-trace.log](loadavg-trace.log) | `/proc/loadavg` every 5 s for the whole 3604 s (721 samples), each line tagged with the last `=== RUN` marker seen, so any sample can be attributed to a run. |

The script's own scratch directory was `/tmp/deck-stability.ItoG43` (its
`run-1.log` … `run-10.log` and `summary.log`); `/tmp` does not survive the run, which
is exactly why all of it is committed here instead of merely cited. Copies of the
four aggregate logs are also in the run's artifacts directory as
`task033-stability-*`.

## 7. Verdict against the requirement

**The rate is 10/10, and the evidence is the directory containing this file:**
`docs/reports/phase3f-033-stability10/` — ten committed per-run logs
([run-1.log](run-1.log) … [run-10.log](run-10.log)), the script's own
[stability-summary.log](stability-summary.log) ending `10/10 passed`, script exit
status `0`, at code sha `0a5034d` from commit `9ce65be`, with all ten start loads and
a 721-sample load trace.

So the phase's stability requirement — `ci/stability.sh 10` green at the final code
commit — **is met at `0a5034d`**, and the earlier 9/10 (task 022, at the previous
approach's `e47cb35`) stands as history rather than as the shipped rate. Nothing here
is rounded up: this section would have named the unmet requirement had the rate been
anything below 10/10, and §4 states the per-failure root-cause obligation that would
have applied.

Two limits on what this measures, stated so no later report over-reads it:

- Ten runs bound a per-run failure rate loosely, not tightly. 10/10 is the
  requirement's bar; it is not evidence that a 1-in-50 flake is absent.
- It measures `0a5034d`. Any later commit that touches compiled code (`*.go`,
  `*.feature`, `*.toml`, `*.sh`, `go.mod`, `go.sum`) invalidates both this run and
  task 032's whole-suite run and obliges a fresh pair; the remaining phase tasks are
  documentation-only for that reason.
