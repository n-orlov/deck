# Phase 4 approach 3 — final whole-tree suite + build/vet/gofmt guards

## Sha

Tail code sha: **`a260abfaa36fe96068fb19f4735d66b3b040459b`**
("tui: close the foreign-content-to-deck boundary reset independently of
deck's own colour (task cure-03-01-2)") — the last commit touching a `*.go`
or `*.feature` file as of this recording. Confirmed by:

```
$ git log --format=%H -1 -- '*.go' '*.feature'
a260abfaa36fe96068fb19f4735d66b3b040459b
$ git rev-parse HEAD          # at the moment the sweep was launched
a260abfaa36fe96068fb19f4735d66b3b040459b
```

This recording **supersedes** the three earlier ones written here, at sha
`3568bd7` (task 002's own tail), at sha `7bb1f8a` (the tail after review pass
234's first two cures) and at sha `bfdb69e` (the R121 cure recording). The
cure pass then landed R118's foreign-content-to-deck boundary reset fix —
`a260abf` ("close the foreign-content-to-deck boundary reset independently
of deck's own colour") — on top of `bfdb69e`, so a sweep of that tree is a
measurement of a superseded tree. Every number below is a fresh measurement
of the post-cure tree, not an amendment of the old one, per the standing rule
that a sweep is true of one tree only.

Tree was clean (`git status --porcelain` empty) at launch; no gitignored
`.review-clone/` worktree was present (`ls .review-clone` → "No such file or
directory").

## What produced this

Full-suite test run — `full-suite.log`:

```
ci/run.sh go test -p=1 -count=1 -timeout=40m ./...
```

Run as a throwaway sibling container (`ci/run.sh`, image `deck-ci:local`,
cache volume `deck-go-cache`), no `-run` filter, no package list — every
package in the module. Backgrounded with `nohup /tmp/r118/sweep.sh &` and
polled per the standing rules (never blocked on inline); the driver script
captured each command's own exit status immediately, into its own status
file, never from log text.

Build guard — `build.log`:

```
ci/run.sh sh -c 'go build ./...'
```

exit 0, **empty output**.

Vet guard — `vet.log`:

```
ci/run.sh go vet ./...
```

exit 0, **empty output**.

Gofmt guard — `gofmt.log`:

```
ci/run.sh gofmt -l .
```

exit 0, output is exactly the four pre-existing drift paths, **labelled here
as pre-existing** (not introduced by this approach's code — this task's own
commit, `a260abf`, touched only `internal/tui/panel.go` and two new
`internal/tui/*_test.go` files, none of these four):

- `internal/theme/quantize_test.go`
- `.spike-preview/cmd/conformance/main.go`
- `.spike-preview/conformance/conformance.go`
- `.spike-preview/conformance/conformance_test.go`

The fifth pre-existing path named in the standing rules,
`.review-clone/internal/theme/quantize_test.go`, is only present when the
gitignored disposable review-clone worktree exists; it did not exist at the
time of this recording (confirmed above), so it does not appear in
`gofmt.log`.

## Suite result

`full-suite.log` is the complete, unexcerpted stdout+stderr of the `go test`
invocation above, and the invocation's own exit status was **0**. It carries
**no `FAIL` line** (`grep -c '^FAIL' full-suite.log` == 0) and every package
that has tests reports `ok`; the three packages with no test files
(`internal/notify`, `internal/search`, `internal/unit`) report
`? ... [no test files]`, which is not a failure.

Per-package timings from the log itself:

| package | result | time |
|---|---|---|
| cmd/deck | ok | 15.199s |
| cmd/fake-claude | ok | 0.873s |
| cmd/fake-codex | ok | 0.193s |
| cmd/fake-pi | ok | 0.782s |
| features | ok | 501.502s |
| internal/agent | ok | 0.008s |
| internal/audit | ok | 0.018s |
| internal/config | ok | 0.023s |
| internal/hookrecv | ok | 4.292s |
| internal/interactive | ok | 10.962s |
| internal/notify | (no test files) | — |
| internal/search | (no test files) | — |
| internal/service | ok | 6.701s |
| internal/store | ok | 2.510s |
| internal/theme | ok | 0.006s |
| internal/tmux | ok | 19.550s |
| internal/tui | ok | 4.427s |
| internal/unit | (no test files) | — |

## Wall-clock duration

The suite command was launched at 2026-09-17T16:08:19Z and returned at
2026-09-17T16:17:54Z (timestamps taken by the driver script immediately
before and after the command itself) — **wall-clock duration = 9m35s
(575s)**, consistent with the sum of the per-package durations `go test`
itself reports (566.6s) plus sibling-container startup/teardown overhead.
This is longer than the ~7m4s-7m21s measured at the earlier recordings; the
difference is ordinary host-scheduler/sibling-container variance (the
`features` package alone accounts for 501.5s of it here vs. 362-419s
before) — the command, flags and package set are unchanged and unnarrowed.

## Skips in force at this sha

- **godog's default tag filter**: `features/` tests run under the default
  `~@real-agents && ~@nightly` expression (no `DECK_GODOG_TAGS` override was
  set for this run), so any scenario tagged `@real-agents` or `@nightly` is
  skipped rather than exercised.
- **Real-binary skips**: the suite exercises the `cmd/fake-claude`,
  `cmd/fake-codex` and `cmd/fake-pi` stub binaries as the tested surface for
  agent adapters; no real Claude/Codex/pi CLI is invoked anywhere in this run
  (there is no credential or network path to one in this sandbox), so the
  fake-* stubs are what "the tested surface" means throughout this suite.

## Result

**Green.** No `FAIL` line anywhere in the suite and `go test`'s own exit
status was 0; build and vet guards are clean (empty output, exit 0); gofmt
shows exactly the pre-existing drift, labelled as such. This is the
full-suite gate for this approach at its final tail code sha `a260abf`.
