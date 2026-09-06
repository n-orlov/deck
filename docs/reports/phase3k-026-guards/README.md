# Phase 3k task 026 — re-verify the three untouched parity guards at the final code sha

> **Superseded 2026-09-06 (task cure-02-02).** An independent review found two remaining probe
> gaps after this run (`lookPathIn` accepted a mode-0644 regular file and a FIFO named like an
> agent's binary); approach 02 cured both (`cure-01-01` `7349dd6`, `201` `c8b00cc`, `202`
> `4e09f2d`), moving the phase's final code sha to `4e09f2de90dcde04bd8fc20c77097e593f2fee5b`.
> This report's guard run at `d88c6625c4ccca71b0d31f7b5864ba030ed39e53` is preserved below
> unchanged as history; it is **not** the gate of record. The current parity-guard evidence is
> `docs/reports/phase3k-206-guards/` (task 206, same guards, current final code sha).

## Final code sha

Last commit touching `*.go` or `*.feature` (matches notes.md):

```
$ git log -1 --format=%H -- '*.go' '*.feature'
d88c6625c4ccca71b0d31f7b5864ba030ed39e53
```

## Guard test run

```
$ ci/run.sh go test -count=1 -v -run 'TestHelpKeymapParity|TestFooterBindingsParity|TestFooterHandlerAgreement' ./internal/tui/
```

Full output captured in [`guards.log`](guards.log); exit status captured in
[`guards.log.exitstatus`](guards.log.exitstatus) (`0`). Final lines:

```
PASS
ok  	github.com/n-orlov/deck/internal/tui	0.012s
```

All three guard tests (`TestHelpKeymapParity`, `TestFooterBindingsParity`,
`TestFooterHandlerAgreement*`) pass, exit 0.

## Untouched-since-baseline check

```
$ git diff --stat 150d7d6..HEAD -- internal/tui/help_keymap_parity_test.go internal/tui/footer_bindings_parity_test.go internal/tui/footer_handler_agreement_test.go
```

Output: **(empty — no lines)**.

Corroborating commit log over the same range and paths, also empty:

```
$ git log --oneline 150d7d6..HEAD -- internal/tui/help_keymap_parity_test.go internal/tui/footer_bindings_parity_test.go internal/tui/footer_handler_agreement_test.go
```

Output: **(empty — no lines)**.

## Disposition

Both commands against the range `150d7d6..HEAD` (`150d7d6` is the phase
baseline referenced throughout the notes, `HEAD` is the current tip at the
time of this run) produce zero output: no commit in this whole run touched
`internal/tui/help_keymap_parity_test.go`,
`internal/tui/footer_bindings_parity_test.go`, or
`internal/tui/footer_handler_agreement_test.go`. This matches the standing
rule that these three guard files "must still pass unedited" — they remain
byte-identical to the phase baseline, and running them fresh at the final
code sha `d88c6625c4ccca71b0d31f7b5864ba030ed39e53` confirms they still pass
(exit 0, all subtests `PASS`). No further action required; this report is
the closing evidence for that standing guard.
