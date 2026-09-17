# Phase 4 approach 3 — final whole-tree suite + build/vet/gofmt guards

## Sha

Tail code sha: **`7bb1f8add502412618ebf4f195b18ffd5536b64a`**
("features: wait for codex's asynchronous first-hook identity adoption before
checking (task cure-03-02)") — the last commit touching a `*.go` or
`*.feature` file as of this recording. Confirmed by:

```
$ git log --format=%H -1 -- '*.go' '*.feature'
7bb1f8add502412618ebf4f195b18ffd5536b64a
$ git rev-parse HEAD
7bb1f8add502412618ebf4f195b18ffd5536b64a
```

This supersedes the record previously written at sha `3568bd7` (task 002's own
tail sha): the two cure tasks, cure-03-01 (settings-footer paint) and
cure-03-02 (real-Codex first-hook wait), landed *.go changes on top of that
sha (commits `2a04e5a` and `7bb1f8a`), so the whole-tree measurement below is
a fresh recording of the new tail, not an amendment of the old one, per the
standing rule that a sweep is true of one tree only.

Tree was clean (`git status --porcelain` empty) before and after this
recording; no gitignored `.review-clone/` worktree was present (`ls
.review-clone` → "No such file or directory").

## What produced this

Full-suite test run — `full-suite.log`:

```
ci/run.sh go test -p=1 -count=1 -timeout=40m ./...
```

Run as a throwaway sibling container (`ci/run.sh`, image `deck-ci:local`,
cache volume `deck-go-cache`), no `-run` filter, no package list — every
package in the module. Backgrounded with `nohup timeout 2700 ... &` and
polled per the standing rules (never blocked on inline).

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
as pre-existing** (not introduced by this approach's code — tasks 001, 002,
cure-03-01 and cure-03-02 touched only `internal/tui/*.go` and
`features/*.go`/`*.feature`, none of these four):

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
invocation above. It carries **no `FAIL` line** (`grep -c '^FAIL'
full-suite.log` == 0) and every package that has tests reports `ok`; the
three packages with no test files (`internal/notify`, `internal/search`,
`internal/unit`) report `? ... [no test files]`, which is not a failure.

Per-package timings from the log itself:

| package | result | time |
|---|---|---|
| cmd/deck | ok | 7.289s |
| cmd/fake-claude | ok | 0.791s |
| cmd/fake-codex | ok | 0.119s |
| cmd/fake-pi | ok | 0.779s |
| features | ok | 364.776s |
| internal/agent | ok | 0.009s |
| internal/audit | ok | 0.019s |
| internal/config | ok | 0.027s |
| internal/hookrecv | ok | 4.433s |
| internal/interactive | ok | 11.270s |
| internal/notify | (no test files) | — |
| internal/search | (no test files) | — |
| internal/service | ok | 6.681s |
| internal/store | ok | 2.560s |
| internal/theme | ok | 0.005s |
| internal/tmux | ok | 19.565s |
| internal/tui | ok | 4.314s |
| internal/unit | (no test files) | — |

## Wall-clock duration

The suite command was launched at 2026-09-17T10:07:38Z and its log file's last
write completed at 2026-09-17T10:14:44Z (`stat` on `full-suite.log` before it
was copied into this directory) — **wall-clock duration ≈ 7m6s (~426s)**,
consistent with the sum of the per-package durations `go test` itself reports
(419.7s) plus sibling-container startup/teardown overhead. This matches the
~441s / 7m21s measured at plan time and the ~412s measured for the previous
(now-superseded) sha; the difference is ordinary variance, not a different
command or a narrowed sweep.

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

**Green.** No `FAIL` line anywhere in the suite; build and vet guards are
clean (empty output); gofmt shows exactly the pre-existing drift, labelled as
such. This satisfies task 003's success criteria at sha `7bb1f8a`.
