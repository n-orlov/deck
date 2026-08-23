# I-1 root cause: which layer loses the keystroke

**Answer: layer 4 — deck's own `Model.Update`/attention-sort state, not keystroke delivery
and not render lag.** It is fixed. Evidence and the exclusion of the other four layers
follow.

## The five candidate layers, and how each was excluded or confirmed

The instrument (task 003/004's `DECK_INPUT_COUNT_FILE` counter, gated by the
`deckinputcount` build tag, `cmd/deck/inputcount_hook.go`) already proves *whether* a byte
reaches Bubble Tea's `Update` dispatch. This task extends it with two more, same pattern,
same tag, no-op unless their env var is set:

- `cmd/deck/rawbytecount_hook.go` / `rawbytecount_default.go` — counts raw bytes at
  `os.Stdin.Read()`, one layer earlier than the existing counter (below Bubble Tea's own
  cancelreader), to separate "never left the PTY" from "left the PTY but Bubble Tea's
  reader lost it".
- `internal/tui/i1trace_hook.go` / `i1trace_default.go` — logs every `tea.Msg`
  `Model.Update` receives (`DECK_I1_TRACE_FILE`), with `m.selected` and the full
  `m.sessions` (index, name, status, `StatusAt`) at entry. This is what actually finds the
  layer: it shows not just *that* a keystroke reached `Update`, but exactly what `Update`
  did with it and why.

Two isolated, full-binary (`ci/run.sh go test ./features/`, no shortcuts) reproductions of
`@requirement-29-bulk-kill` failing were captured with `DECK_I1_TRACE_FILE` set, at load
average 2.6–2.9 (`docs/reports/phase3d-i1-rootcause-update-trace-1.log`,
`...-update-trace-2.log`).

1. **Layer 1 (harness `Send` never reaching the pty) — excluded.** godog's own send log
   shows every `Send` call returning success at the recorded timestamp for every attempt.
2. **Layer 2 (pty/kernel buffer drop) — excluded, cheaply, as the PRD expected.** PTYs do
   not drop bytes under back-pressure; nothing in either trace shows a byte written and
   never read.
3. **Layer 3 (Bubble Tea v1.3.10's reader losing a byte) — excluded by direct
   measurement.** Both traces show `KeyMsg("j")` entering `Model.Update` in every single
   reproduction, at `m.selected=1`, immediately after `KeyMsg("m")` and with no other
   `tea.Msg` intervening. The byte is read off stdin, decoded into exactly one `tea.KeyMsg`,
   and dispatched. Task 004's original single-sample read (`bk-one running [marked]` "142
   → flat at 143") that suggested a layer-1/3 drop was a **thinned-log artifact**: its
   poll-on-render counter log
   (`docs/reports/phase3d-i1-repro-counter-poll.log`) was thinned 3x for size, and at that
   coarser granularity `m`'s own `+1` and `j`'s own `+1` land in the *same* ~39 ms sample
   window and are visually indistinguishable from one `+2` jump followed by a flat line.
   Every one of this task's un-thinned, per-message traces shows the two keystrokes
   incrementing separately and `j` unambiguously reaching `Update`.
4. **Layer 4 (deck's own `Model.Update` — a real product defect) — confirmed, this is the
   answer.** See mechanism below.
5. **Layer 5 (render lag, the keystroke was processed and the assertion was just too
   fast) — excluded.** `m.selected` in the trace **never changes again** for the rest of
   either failing run, all the way to the 5 s timeout (both full traces are committed).
   There is nothing to wait for: `Update` ran the `"down", "j"` case to completion,
   synchronously, and it correctly found no next row to move to, given the state at that
   instant. Waiting longer changes nothing.

## The mechanism (layer 4)

`internal/tui/tui.go`'s `sessionsLoaded` handler re-sorts `m.sessions` by
`sortSessionsByAttention` (SPEC requirement 28/29: rank, then `StatusAt` ascending, then —
`internal/tui/attention.go`'s own documented tie-break, not a SPEC.md requirement — session
ID ascending) on every load, and re-resolves the selected row **by session ID**, so the same
session stays selected across a resort. That part is correct and is not what fails.

The three `@requirement-29-*` fixtures all create two sessions moments apart, then act on a
mark set spanning both. `internal/service/reconcile.go` promotes a shell from `starting` to
`running` the first pass it observes the tmux pane alive, stamping `StatusAt` with
`s.Clock.Now().UnixMilli()` — millisecond resolution. Both traces show exactly this:

```
enter sessionsLoaded[bk-one:starting@...026 bk-two:starting@...139]   <- distinct StatusAt, order stable
enter sessionsLoaded[bk-one:running@...160 bk-two:running@...160]     <- IDENTICAL StatusAt (same reconcile pass)
```

When both sessions are promoted in the **same reconcile pass**, both calls to
`s.Clock.Now().UnixMilli()` round to the same millisecond. Rank is now equal (both
`running`) and `StatusAt` is now equal too, so `sortSessionsByAttention`'s only remaining
key is session ID — a random UUID (`internal/config.NewIDGenerator`), uncorrelated with
creation order, with which row was just marked, or with which row a `k`/`j` keystroke was
aimed at. In both captured failures `bk-two`'s ID happened to sort before `bk-one`'s,
silently swapping the two rows' screen positions between the `k`/`m` and the `j` — the
selected (and just-marked) `bk-one` becomes the sidebar's **last** row, so `"down"`'s own
`nextVisibleSelection` correctly, synchronously finds no next row and no-ops. This is
*exactly* the PRD's failure signature: "a single, non-coalesced `j` ... never takes effect,
the selection stays on the first marked row."

This is not a SPEC violation to fix by changing the sort's tie-break: SPEC.md's own text
(`Sort: waiting (oldest first) → error → running → starting → idle → stopped`) says nothing
about ID at all — the ID tie-break is `attention.go`'s own addition for the case
`sortSessionsByAttention` actually needs it (a pure function with no memory of any earlier
frame, e.g. the very first load of two sessions created in the same millisecond), and
`TestSortSessionsByAttentionTiesBrokenByID` (an existing, requirement-29-labelled test) locks
that case in. It is untouched.

## The fix (`internal/tui`, product code)

`internal/tui/attention.go` adds `sortSessionsByAttentionStable(previous, incoming)`: the
same rank/`StatusAt` total order as `sortSessionsByAttention`, but for a genuine tie on
*both* keys, it prefers the pair's own relative order in `previous` (the sidebar's last
frame) over the ID coin flip, when both sessions appeared in that previous frame. A tie with
no shared previous frame (both brand new this load) still falls back to the exact same ID
order `sortSessionsByAttention` uses — `sortSessionsByAttention`/`lessByAttention` themselves
are unmodified, and their existing tests (`TestSortSessionsByAttentionTiesBrokenByID`
included) still pass unchanged.

`internal/tui/tui.go`'s `sessionsLoaded` handler now calls
`sortSessionsByAttentionStable(m.sessions, msg.sessions)` instead of the plain
`sortSessionsByAttention(msg.sessions)`. Once two sessions have appeared together in any
earlier frame — which, in the marked-set idiom, they always have by the time a `k`/`m`/`j`
sequence runs — a later tie (a same-millisecond promotion) can no longer invert their order.

New unit tests in `internal/tui` (`internal/tui/i1_marked_nav_test.go`), red without the
fix (confirmed: reverting the `sessionsLoaded` wiring to plain `sortSessionsByAttention`
reproduces the exact same failure the tests assert against — `after k,m,j selected="bk-one",
want bk-two`):

- `TestSortSessionsByAttentionStablePrefersPreviousOrderOverIDCoinFlip` — pure-function
  proof that a same-rank/same-`StatusAt` tie keeps the previous frame's order instead of the
  ID coin flip.
- `TestSortSessionsByAttentionStableFallsBackToIDWithNoPreviousFrame` — proof the no-history
  fallback still matches `sortSessionsByAttention` exactly.
- `TestMarkedSetKMJSurvivesBothSessionsRacingToRunningTogether` — the exact k/m/j idiom at
  the `Model.Update` level, with the two sessions' IDs deliberately picked so the ID-only
  sort would flip them; asserts `j` lands on the second session, never stuck on the first
  marked row.

## Stability evidence: 10/10 consecutive isolated runs, each scenario

Same host, same day, load average 1.9–5.0 (comparable to task 004's own reproduction range
of 2.2–7.3; the original Phase 3 measurement this task is closing was recorded at 6.18–7.28).

| scenario | before this fix | after this fix |
|---|---|---|
| `@requirement-29-bulk-kill` | 3/10 (Phase 3) | **10/10** — `docs/reports/phase3d-i1-rootcause-stability-bulk-kill.log` |
| `@requirement-29-bulk-delete` | 5/10 (Phase 3) | **10/10** — `docs/reports/phase3d-i1-rootcause-stability-bulk-delete.log` |
| `@requirement-29-batch-undo` | 8/10 (Phase 3) | **10/10** — `docs/reports/phase3d-i1-rootcause-stability-batch-undo.log` |

Each log is ten consecutive isolated `ci/run.sh go test ./features/` invocations (fresh
process, fresh scenario state each time — never a shared runner across attempts), tagged to
the one scenario, with `/proc/loadavg` recorded at the top of every attempt.

## Forbidden closes — re-checked against this task's diff

- No timeout widened, no sleep added, no retry added, no `@flaky`/`@nightly` tag, no
  checkpoint removed, no marked set shrunk, no marking by any route other than real
  keystrokes, no scenario deleted. `git diff` for this task touches only
  `internal/tui/attention.go`, `internal/tui/tui.go`, `internal/tui/i1_marked_nav_test.go`,
  `cmd/deck/rawbytecount_hook.go`, `cmd/deck/rawbytecount_default.go`, `cmd/deck/main.go`,
  `internal/tui/i1trace_hook.go`, `internal/tui/i1trace_default.go`, and this report plus its
  companion logs. No `.feature` file, no `SPEC.md`, no `ci/`, no `prds/` file is touched.
- The two new instruments (`rawbytecount_*`, `i1trace_*`) are, like task 003/004's own
  counter, gated behind the `deckinputcount` build tag and a specific env var, no-ops in
  every ordinary build and every ordinary run; `go vet`/`go build`/`gofmt -l` and the full
  `go test ./...` (including `features`) pass with and without the tag.
