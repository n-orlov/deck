# I-11 / requirement 40 — the store durability contract, decided and proven

## The contract, in SPEC's own terms

- **R3 ("Durable identity")**, SPEC.md:44: "A named session and its
  conversation outlive a reboot, an agent upgrade, and `tmux kill-server`.
  Nothing is auto-restarted; resume is one keypress, on demand." — a claim
  about what the *store* remembers, independent of tmux.
- **R4 ("N concurrent TUIs, one host")**, SPEC.md:45: "State lives in tmux +
  SQLite (WAL). No process is authoritative, no process is required. …
  every mutation is a targeted `UPDATE`." WAL journal mode is deck's chosen
  mechanism; `internal/store.OpenPath` sets `journal_mode=WAL`,
  `synchronous=NORMAL`, `busy_timeout=5000`, `foreign_keys=ON`
  (`internal/store/store.go:100`).
- **The explicit boundary of the guarantee**, SPEC.md:1550-1557 (§13.2,
  "Reboot"): "Real reboots don't belong in CI. `tmux kill-server -L
  <socket>` is the in-suite equivalent … A genuine power-cycle check stays a
  tagged nightly/manual scenario, since only that catches `fsync`-level
  loss."

Put together: deck's durability contract is **crash-consistency against an
OS-level process crash** (SIGKILL, panic, OOM-kill) — every mutation that
also needs a paired audit-log row (`internal/store.Store.
mutateSessionWithEvent`, the engine behind `SetPermissionProfile` and most
other `Set*` methods) is issued as one transaction, so a crash before
`Commit` leaves neither write behind. It is **explicitly not** a claim about
surviving a real power loss: `synchronous=NORMAL` does not fsync on every
commit, so the last WAL frame(s) can still be lost to a power cut even
though they would have survived a mere process crash. SPEC.md's own words
scope that case out of CI and into a tagged nightly/manual scenario — this
task proves the process-crash half, which is the half a container can
reach, not the power-cycle half.

## The proof

`internal/store/durability_test.go`'s `TestStoreSurvivesProcessCrashMidTransaction`
(self-exec crash helper `TestStoreDurabilityCrashHelper`, same pattern as
`internal/tmux/tmux_test.go`'s `TestAttachHelper`):

1. **`atomic transaction killed before Commit leaves nothing behind`** — a
   real, separately-killable OS process opens the store, begins a
   transaction, executes the exact two statements `mutateSessionWithEvent`
   issues (`UPDATE sessions SET permission_profile = ?`, then `INSERT INTO
   events (...)`), signals ready, and blocks *without ever calling `Commit`
   or `Rollback`*. The parent test waits for the ready signal, sends
   `SIGKILL`, waits for the process to exit, and reopens the same database
   file fresh. Assertions: `permission_profile` is still `"safe"` (the
   UPDATE never took effect) and there are zero matching `events` rows (the
   INSERT never took effect) — both halves of the transaction are gone,
   together, exactly as SQLite's WAL rollback journal for a never-committed
   transaction guarantees.
2. **`weakened autocommit path killed between the two writes leaves an
   inconsistent partial write`** — the same two statements, but each its own
   autocommit execution with no shared transaction: the hazard a shared
   transaction exists to prevent. Killed between the two, the first
   statement (already durable the instant it returned) survives
   (`permission_profile == "yolo"`) while the second never ran
   (`events` count `0`). This is a genuinely inconsistent partial write,
   not a rollback — proof the test can distinguish "the contract held" from
   "the contract was violated" rather than passing regardless of outcome.

A shared `assertPairAtomic` checker (the atomicity invariant itself: mutated
⟺ logged) was applied to **both** scenarios as a one-off demonstration
before being removed from the weakened scenario:

- Applied to the weakened scenario, it fails —
  [`phase3d-i11-store-durability-red-demo.log`](phase3d-i11-store-durability-red-demo.log)
  (`exit=1`, `atomicity violated: permission_profile mutated=true (now
  "yolo") but paired event logged=false (0 rows)`). This is the red proof
  the task asks for: not a comment, an assertion that actually ran and
  failed against the deliberately weakened (non-transactional) commit path.
- That temporary call was reverted before commit (the weakened scenario's
  own explicit assertions describe the outcome instead, so the committed
  test is green); it is kept permanently on the atomic scenario, where it
  passes because both halves are absent together.

Final green run —
[`phase3d-i11-store-durability-run.log`](phase3d-i11-store-durability-run.log):

```
=== RUN   TestStoreSurvivesProcessCrashMidTransaction
=== RUN   TestStoreSurvivesProcessCrashMidTransaction/atomic_transaction_killed_before_Commit_leaves_nothing_behind
=== RUN   TestStoreSurvivesProcessCrashMidTransaction/weakened_autocommit_path_killed_between_the_two_writes_leaves_an_inconsistent_partial_write
--- PASS: TestStoreSurvivesProcessCrashMidTransaction (0.08s)
    --- PASS: TestStoreSurvivesProcessCrashMidTransaction/atomic_transaction_killed_before_Commit_leaves_nothing_behind (0.04s)
    --- PASS: TestStoreSurvivesProcessCrashMidTransaction/weakened_autocommit_path_killed_between_the_two_writes_leaves_an_inconsistent_partial_write (0.04s)
PASS
ok  	github.com/n-orlov/deck/internal/store	0.084s
```

Verified alongside: `ci/run.sh go build ./...`, `go vet ./...`, `gofmt -l
$(git ls-files '*.go')` clean; `ci/run.sh go test -count=1 ./internal/...
./cmd/...` green (all packages, `internal/store` included). No full-suite
run this task — a targeted package run is sufficient evidence for a single
new test file touching no other package. `SPEC.md` is unmodified
(`git diff -- SPEC.md` empty).
