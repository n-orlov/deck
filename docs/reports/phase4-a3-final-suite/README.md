# Phase 4 approach 3 — final whole-tree suite + build/vet/gofmt guards

## Sha

Tail code sha: **`3568bd7971a782fadbf589d79ce5777c0f1b5315`**
("tui: paint the interactive preview branch's own notice and pad rows (task 002)")
— the last commit touching a `*.go` or `*.feature` file as of this recording
(task 002, the last code-touching task in this approach). Confirmed by:

```
$ git log --format=%H -1 -- '*.go' '*.feature'
3568bd7971a782fadbf589d79ce5777c0f1b5315
$ git rev-parse HEAD
3568bd7971a782fadbf589d79ce5777c0f1b5315
```

Tree was clean (`git status --porcelain` empty) before and after this recording;
no gitignored `.review-clone/` worktree was present.

## What produced this

Full-suite test run — `full-suite.log`:

```
ci/run.sh go test -p=1 -count=1 -timeout=40m ./...
```

Run as a throwaway sibling container (`ci/run.sh`, image `deck-ci:local`, cache
volume `deck-go-cache`), no `-run` filter, no package list — every package in
the module. Backgrounded with `nohup timeout 2700 ... &` and polled per the
standing rules (never blocked on inline).

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

exit 0, output is exactly the four pre-existing drift paths, **labelled here as
pre-existing** (not introduced by this approach's code — tasks 001/002 touched
only `internal/tui/*.go`, none of these four):

- `internal/theme/quantize_test.go`
- `.spike-preview/cmd/conformance/main.go`
- `.spike-preview/conformance/conformance.go`
- `.spike-preview/conformance/conformance_test.go`

The fifth pre-existing path named in the standing rules,
`.review-clone/internal/theme/quantize_test.go`, is only present when the
gitignored disposable review-clone worktree exists; it did not exist at the
time of this recording (confirmed above), so it does not appear in `gofmt.log`.

## Suite result

`full-suite.log` is the complete, unexcerpted stdout+stderr of the `go test`
invocation above. It carries **no `FAIL` line** (`grep -c '^FAIL' full-suite.log`
== 0) and every package that has tests reports `ok`; the three packages with no
test files (`internal/notify`, `internal/search`, `internal/unit`) report `?
... [no test files]`, which is not a failure.

Per-package timings from the log itself:

| package | result | time |
|---|---|---|
| cmd/deck | ok | 7.237s |
| cmd/fake-claude | ok | 0.783s |
| cmd/fake-codex | ok | 0.119s |
| cmd/fake-pi | ok | 0.774s |
| features | ok | 351.958s |
| internal/agent | ok | 0.009s |
| internal/audit | ok | 0.018s |
| internal/config | ok | 0.028s |
| internal/hookrecv | ok | 4.374s |
| internal/interactive | ok | 10.784s |
| internal/notify | (no test files) | — |
| internal/search | (no test files) | — |
| internal/service | ok | 6.543s |
| internal/store | ok | 2.556s |
| internal/theme | ok | 0.006s |
| internal/tmux | ok | 19.423s |
| internal/tui | ok | 4.204s |
| internal/unit | (no test files) | — |

## Wall-clock duration

The suite command was launched at 2026-09-17T05:31:29Z and its log file's last
write completed at 2026-09-17T05:38:21Z (`stat` on `full-suite.log` before it
was copied into this directory) — **wall-clock duration ≈ 6m52s (~412s)**,
consistent with the sum of the per-package durations `go test` itself reports
(408.8s) plus sibling-container startup/teardown overhead. This matches the
~441s / 7m21s measured for the same command earlier in this approach (warm
cache); the small difference is ordinary variance, not a different command or
a narrowed sweep.

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
such. This satisfies task 003's success criteria at sha `3568bd7`.
