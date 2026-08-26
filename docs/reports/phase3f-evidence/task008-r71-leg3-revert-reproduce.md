# Task 008 (R71 leg 3) — would these tests go red if the fix were reverted?

Fix under test: hook resolution reads the new `store.ListSessionsIncludingArchived`
(`WHERE deleted_at = 0`, archived rows included) instead of `store.ListSessions`
(`WHERE deleted_at = 0 AND archived_at = 0`, the sidebar's display query).
Both experiments mutate a product file in the live workspace, run, then restore from a
pre-mutation copy in `/tmp` and prove the tree came back byte-identical.

## Green baseline (loadavg 6.97/3.76/3.21)

    ci/run.sh go test -count=1 ./internal/hookrecv/ ./internal/store/
    ok  github.com/n-orlov/deck/internal/hookrecv  4.690s
    ok  github.com/n-orlov/deck/internal/store     3.037s

    ci/run.sh go vet ./...        # clean, no output

## Experiment 1 — the naive implementation this task replaces (loadavg 9.06/4.48/3.46)

`internal/hookrecv/receiver.go`: `resolve` reads `db.ListSessions(ctx)` again and the
`Store` interface asks for `ListSessions` (the pre-task state, compiling and plausible —
this is exactly the code shipped before this commit).

    ci/run.sh go test -count=1 ./internal/hookrecv/ ./internal/store/
    --- FAIL: TestReceiveResolvesAnArchivedRowByBothKeys (0.03s)
        archived_resolution_test.go:44: conversation-id hook on an archived row: hook
        session could not be resolved (conversation_id="archived-conversation"
        injected_session_id="")
    FAIL  github.com/n-orlov/deck/internal/hookrecv  4.690s
    ok    github.com/n-orlov/deck/internal/store     3.106s

That failure message is issue #8's captured pane banner in miniature: a correct
conversation id, unresolvable. The negative test (`...KeepsATombstonedRowsHookAnOrphan`)
passes under this revert, as it must — it guards the opposite mistake.

## Experiment 2 — the LAZY fix the PRD warns about (same loadavg window)

`internal/store/store.go`: `ListSessionsIncludingArchived`'s `WHERE deleted_at = 0` term
dropped entirely (`FROM sessions ORDER BY created_at, id`) — i.e. "all rows", which is
what dropping both conditions instead of one looks like.

    ci/run.sh go test -count=1 -run 'TestReceiveKeepsATombstonedRowsHookAnOrphan|TestListSessionsIncludingArchived' ./internal/hookrecv/ ./internal/store/
    --- FAIL: TestReceiveKeepsATombstonedRowsHookAnOrphan/by_conversation_id
        tombstoned hook = Result{SessionID:"tombstoned-row", Status:"idle", Kind:"stop",
        Orphan:false}, err <nil>; want unresolved orphan
    --- FAIL: TestReceiveKeepsATombstonedRowsHookAnOrphan/by_injected_row_id
        tombstoned hook = Result{SessionID:"tombstoned-row", ... Orphan:false}; want unresolved orphan
    FAIL  github.com/n-orlov/deck/internal/hookrecv  0.030s
    --- FAIL: TestListSessionsIncludingArchivedKeepsArchivedAndDropsTombstoned
        ListSessionsIncludingArchived = [archived archived-then-deleted plain tombstoned],
        want [archived plain]
    FAIL  github.com/n-orlov/deck/internal/store  0.028s

So each half of the accessor's `WHERE` clause is pinned in the direction it matters:
dropping `archived_at = 0` is required (experiment 1) and keeping `deleted_at = 0` is
required (experiment 2).

## Restore proof

    cp /tmp/receiver.go.keep internal/hookrecv/receiver.go   # after experiment 1
    cp /tmp/store.go.keep    internal/store/store.go         # after experiment 2
    ci/run.sh go test -count=1 ./internal/hookrecv/ ./internal/store/
    ok  github.com/n-orlov/deck/internal/hookrecv  4.744s
    ok  github.com/n-orlov/deck/internal/store     3.163s

    git status --short
     M internal/hookrecv/receiver.go
     M internal/store/archive_test.go
     M internal/store/store.go
    ?? internal/hookrecv/archived_resolution_test.go

— exactly this task's own three edits plus its new test file, no experiment residue.

## `reconcile`'s use of `ListSessions` is deliberately UNCHANGED

    git diff internal/service/reconcile.go   # 0 lines

Asymmetry rationale (also in the commit message): once R71 leg 1 refuses resume/restart on
an archived row, an archived row cannot own a live pane, so excluding it from
reconciliation is correct — and including it would let the reconciler WRITE statuses onto
archived rows, a new bug. The hook path is different because a hook write is evidence
about a row that exists, and `store.UpdateSessionStatus` already refuses to let a hook
resurrect a `stopped` row (`internal/store/store.go`'s status guard), so a late hook lands
on the right row without reviving it.
