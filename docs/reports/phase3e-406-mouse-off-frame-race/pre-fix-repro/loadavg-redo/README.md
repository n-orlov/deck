# Loadavg-instrumented redo of the pre-fix reproduction sweep

Validation on the original task 406 report found that `README.md` §6 claimed "Both
loads recorded via `uptime` immediately before each run" and pointed at "the
individual `README.md` files under `pre-fix-repro/` and `post-fix-sweep-scoped/`
for the full per-run table" — but neither sub-directory's table, nor any `*.log`
file under the report, actually contained a per-run loadavg value. That claim was
false as written. This directory is the correction: the same 6-delay x 3-trial =
18-run sweep, re-run against the mechanism (mouse.feature's DECK_MOUSE=0 scenario
manually reverted to the pre-406 `captures its frame as` step for the duration of
this redo only, restored immediately after — `git diff features/mouse.feature` is
empty in the commit that carries this directory) with a real `uptime` sample taken
in the calling shell immediately before every single invocation.

Contention was induced the same way as the original sweep (deliberately-spun
`busybox sh -c 'while true; do :; done'` sibling containers, `--label
ralphd.role=sibling` + `--label deck.exp=406redo`, stopped again — by the captured
container IDs, never by pattern — before this redo's commit), sized down from an
initial 50 (which pushed 1-minute loadavg over 60 and started producing *unrelated*
timeouts — `client "A" did not show "idle" within 500ms`, not this mechanism's
`frame changed after gesture` — because the harness's own step budgets are tight
enough that extreme contention starves other steps too) to a steady ~20 containers,
which reliably reproduced the exact review signature without that noise.

## Per-run table (`table.txt`, this directory)

```
delay=0.05 trial=1 exit=1 loadavg_before=21.05, 33.34, 44.05   <- FAIL, log attached
delay=0.05 trial=2 exit=0 loadavg_before=24.59, 33.70, 44.06
delay=0.05 trial=3 exit=0 loadavg_before=26.86, 34.03, 44.11
delay=0.1  trial=1 exit=0 loadavg_before=27.99, 34.03, 44.00
delay=0.1  trial=2 exit=1 loadavg_before=29.32, 34.11, 43.92   <- FAIL, log attached
delay=0.1  trial=3 exit=0 loadavg_before=31.04, 34.25, 43.80
delay=0.2  trial=1 exit=0 loadavg_before=33.15, 34.59, 43.81
delay=0.2  trial=2 exit=1 loadavg_before=35.62, 35.06, 43.82   <- FAIL, log attached
delay=0.2  trial=3 exit=1 loadavg_before=37.02, 35.37, 43.82   <- FAIL, log attached
delay=0.3  trial=1 exit=0 loadavg_before=38.74, 35.83, 43.84
delay=0.3  trial=2 exit=0 loadavg_before=39.66, 36.11, 43.84
delay=0.3  trial=3 exit=0 loadavg_before=39.49, 36.25, 43.77
delay=0.5  trial=1 exit=0 loadavg_before=38.91, 36.24, 43.68
delay=0.5  trial=2 exit=0 loadavg_before=36.76, 35.90, 43.45
delay=0.5  trial=3 exit=0 loadavg_before=38.26, 36.25, 43.48
delay=0.7  trial=1 exit=0 loadavg_before=37.40, 36.13, 43.37
delay=0.7  trial=2 exit=0 loadavg_before=38.46, 36.42, 43.34
delay=0.7  trial=3 exit=0 loadavg_before=38.26, 36.44, 43.28
```

`loadavg_before` is the exact `uptime` 1/5/15-minute triple sampled in the calling
shell in the instant before that row's `ci/run.sh ... go test ...` invocation
started (1-minute figure is the most representative of contention *at* the run,
since it is the least lagged of the three).

**4/18 reproduce the mechanism** (more than the original, uninstrumented sweep's
2/18 — a real, not cherry-picked, difference; this redo used a different induced
contention level, `~21-39` 1-minute loadavg vs. the original's claimed `~55-60`,
and reproduction rate is not claimed to be comparable across the two). All 4
failure logs are attached in this directory (`prefix-delay0.05-trial1.log`,
`prefix-delay0.1-trial2.log`, `prefix-delay0.2-trial2.log`,
`prefix-delay0.2-trial3.log`); each shows the exact review signature:

```
$ grep -n 'Error:' prefix-delay0.05-trial1.log
194:      Error: after scenario hook failed: client "A" frame changed after gesture, want unchanged from captured "before-mouse-off-click":
```

The pre-existing `../repro-1-delay0.05s-trial3.log` and `../repro-2-delay0.2s-trial1.log`
(no loadavg attached — the original, uninstrumented sweep this redo corrects) remain
in the parent directory as the historical record of the first reproduction; they are
superseded as *loadavg* evidence by this directory, not deleted.
