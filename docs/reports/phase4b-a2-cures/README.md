# Phase 4b approach 2 — cure attestation for B1, B2, B3, R1 (task 006)

This report attests that the four review findings this approach set out to
cure (B1, B2, B3, R1 — tasks 001-004) are in fact cured: for each, the commit
sha that cured it and the Go test function that proves it, plus one targeted
re-run covering exactly those four test functions, taken at the tree task
005 pinned as this approach's measurement anchor.

## The four findings

| finding | cure commit | test function that proves it | file |
| --- | --- | --- | --- |
| B1 | `113b552ca2fe42bb1b3d10e42f14cdc3f735c8e6` | `TestFilterByDefaultMatchesTheSidebarsOwnDefaultLabel` | `internal/tui/filter_default_label_test.go` |
| B2 | `075c59c35fbbc22461f748bbe87e87a8ac4139e4` | `TestSettingsGroupsPanelRefreshesOnOrdinaryReload` | `internal/tui/settings_groups_reload_test.go` |
| B3 | `00f33a5ac204bc2f558352a18c40fefa24ad3bab` | `TestDisplacementFallbackReseedLeavesNoArtificialScrollbackOrCue` | `internal/tui/interactive_displacement_scrollcue_test.go` |
| R1 | `70c7430df3b23a46fb735e8573e26ec55908adeb` | `TestEmptyAndHelpViewsAreDiscoverable` | `internal/tui/tui_test.go` |

Each cure commit's own message names the finding's root cause, its fix, and
(for B1/B2/B3) the new regression test it adds; for R1 the cited function is
one of the two pre-existing pinned assertions the commit re-worded (the other
lives in `cmd/deck/main_test.go`'s `TestDeckBinaryEmptyHelpAndQuitThroughPTY`,
named here for completeness but not one of the four test functions the
targeted re-run below covers — the criterion asks for exactly one function
per finding, and `TestEmptyAndHelpViewsAreDiscoverable` is the one that
directly renders the corrected help line). All four commits and all four
test names are re-derived from the commits themselves (`git show <sha>:<file>
| grep '^func Test'`), not copied from an earlier report.

All four test functions happen to live in the same package, `internal/tui`.

## One targeted re-run covering exactly those four test functions

```
$ ci/run.sh go test -count=1 -v -run '^(TestFilterByDefaultMatchesTheSidebarsOwnDefaultLabel|TestSettingsGroupsPanelRefreshesOnOrdinaryReload|TestDisplacementFallbackReseedLeavesNoArtificialScrollbackOrCue|TestEmptyAndHelpViewsAreDiscoverable)$' ./internal/tui
```

All four PASS, package exit 0. Full committed output:
`targeted-rerun-b1-b2-b3-r1.log`.

## The tree that re-run was taken at

The re-run above was executed against the working tree checked out at commit
`e9aba76d12654ec71a60a00a5f5e748faa8dbd8c` — task 005's own retake-addendum
commit, the current `HEAD` at the start of this task and the freeze's
record-only state (no `.go`/`.feature` file has changed since). That commit
already existed before this task started, so citing its sha here is not a
self-reference.

The four tree-object hashes read at that commit, each labelled by path:

| path | tree-object hash at `e9aba76…` | task 005's pinned hash for the same path | pair |
| --- | --- | --- | --- |
| `internal` | `15506734d4989e111e871a419ebf46c94a3b59a3` | `15506734d4989e111e871a419ebf46c94a3b59a3` | EQUAL |
| `cmd` | `27ff2eba72ef6a63a6cf49285c4ddc6660b7fb0d` | `27ff2eba72ef6a63a6cf49285c4ddc6660b7fb0d` | EQUAL |
| `features` | `b5dbe2f565eb96b2654f8d1d64fcc51eb711f60d` | `b5dbe2f565eb96b2654f8d1d64fcc51eb711f60d` | EQUAL |
| `ci` | `0a183631a2beea070ab0f7d8fa027aecf423e7b0` | `0a183631a2beea070ab0f7d8fa027aecf423e7b0` | EQUAL |

Read with `git rev-parse e9aba76d12654ec71a60a00a5f5e748faa8dbd8c:<path>` for
the left column; the right column is copied verbatim from
`docs/reports/phase4b-a2-code-revision.md`, task 005's own pin.

## The pytest verification of every pair

The criterion asks for the pair equality to be verified by rerunning pytest.
The harness is committed beside this report as a plain listing,
`pytest-tree-hash-equal.py.txt` (the `.py.txt` suffix keeps it uncollectable
by the repo's own tests — the deck repo's test surface stays Go test
functions and godog scenarios only, and no test framework is added to it).
It asserts, for each of the four paths, that `git rev-parse
<DECK_READ_COMMIT>:<path>` (default `e9aba76d12654ec71a60a00a5f5e748faa8dbd8c`,
the commit the re-run above was taken at) equals the hash task 005 pinned for
that path.

Re-run it:

```
mkdir -p /tmp/verify006
cp docs/reports/phase4b-a2-cures/pytest-tree-hash-equal.py.txt \
   /tmp/verify006/test_cure_attestation.py
python3 -m pytest --version                 # pip install --user pytest, if absent
DECK_REPO=/workspace python3 -m pytest -v -p no:cacheprovider \
   /tmp/verify006/test_cure_attestation.py
```

Result: 4 passed, pytest exit 0. Committed output: `pytest-tree-hash-equal.log`.

## `verify-citations.sh`

`verify-citations.sh`, committed in this same directory, enumerates exactly
the four shas and four test names this README cites above and checks each
resolves at the commit it is run from:

- each sha resolves as a commit object (`git cat-file -e <sha>^{commit}`);
- each named test function is defined in the named file, at that sha
  (`git show <sha>:<file> | grep '^func <name>('`).

Run it:

```
sh docs/reports/phase4b-a2-cures/verify-citations.sh
```

All eight checks (four commits, four test functions) pass; exit 0. Committed
output: `verify-citations.log`.

## Prior evidence this attestation summarizes

The per-finding red/green logs recorded when each finding was cured
(tasks 001-004) remain in their own sibling directories and are not repeated
here: `b1/`, `b2/`, `b3/`, `r1/`.
