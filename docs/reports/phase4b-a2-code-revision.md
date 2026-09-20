# Phase 4b approach 2 — pinned code revision (task 005)

This record pins the code revision that every downstream measurement task in
approach 2 (006-012) measures against. The freeze line takes effect from this
task's launch onward: task 004 (R1) was the last code-touching task, so every
commit from here on is record-only (`docs/reports/**`, `docs/DELIVERY-LOG.md`,
`docs/PLAN.md`).

## Pinned revision

```
$ git log -1 --format=%H -- '*.go' '*.feature'
70c7430df3b23a46fb735e8573e26ec55908adeb
```

`REV = 70c7430df3b23a46fb735e8573e26ec55908adeb` — the latest commit that
touches a `.go` or `.feature` file, i.e. finding R1's cure commit. This is
the tree every downstream measurement task (006-012) cites as the identity
of the code it ran against.

## Tree-object hashes at REV

Read with `git rev-parse <REV>:<path>` for the four paths downstream tasks
cite:

| path | tree-object hash at REV |
| --- | --- |
| `internal` | `15506734d4989e111e871a419ebf46c94a3b59a3` |
| `cmd` | `27ff2eba72ef6a63a6cf49285c4ddc6660b7fb0d` |
| `features` | `b5dbe2f565eb96b2654f8d1d64fcc51eb711f60d` |
| `ci` | `0a183631a2beea070ab0f7d8fa027aecf423e7b0` |

## The same four hashes read at this task's own commit — each pair equal

The proof that this record-only commit did not move code is a *pair* per
path: the tree-object hash of that path **at REV** against the tree-object
hash of the same path **at the commit being read at** (`git rev-parse
<commit>:<path>`, not `<REV>:<path>` twice). Four paths, four pairs, each
pair equal means the commit that publishes this record carries exactly REV's
`internal`, `cmd`, `features` and `ci` trees — no code moved.

| path | at REV | at this task's own publishing commit (`154c63e1…`) | pair |
| --- | --- | --- | --- |
| `internal` | `15506734d4989e111e871a419ebf46c94a3b59a3` | `15506734d4989e111e871a419ebf46c94a3b59a3` | EQUAL |
| `cmd` | `27ff2eba72ef6a63a6cf49285c4ddc6660b7fb0d` | `27ff2eba72ef6a63a6cf49285c4ddc6660b7fb0d` | EQUAL |
| `features` | `b5dbe2f565eb96b2654f8d1d64fcc51eb711f60d` | `b5dbe2f565eb96b2654f8d1d64fcc51eb711f60d` | EQUAL |
| `ci` | `0a183631a2beea070ab0f7d8fa027aecf423e7b0` | `0a183631a2beea070ab0f7d8fa027aecf423e7b0` | EQUAL |

Raw pair reads, both sides of every pair spelled out with the command that
produced them:

- before this task's own commit existed (read at `485f9b547a096895c609211e4936aeda888b3b9d`,
  the previous record-only commit):
  `docs/reports/phase4b-a2-code-revision/tree-hash-pairs-pre-commit.log`
- at this task's own publishing commit (`154c63e109e4cfebd9586eae8c5328eeee823930`):
  `docs/reports/phase4b-a2-code-revision/tree-hash-pairs-post-commit.log`
  (read after that commit existed, and committed by this task's addendum
  commit — the only reason the task has two commits is that a commit's own sha
  cannot be quoted before it exists).

Both logs also carry `git diff --name-only REV <commit>` as corroboration
(never as the identity proof, which is the equal pairs above): the only paths
it lists are under `docs/reports/`, no `.go` and no `.feature` file.

## The pytest verification of every pair

The criterion asks for the pair equality to be verified by rerunning pytest.
The harness that does it is committed beside this report as a plain listing,
`docs/reports/phase4b-a2-code-revision/pytest-tree-hash-pairs.py.txt`, with a
runnable copy in this run's artifacts directory at
`/run/ralphd/artifacts/a2-005-pytest/test_pinned_revision.py`. It asserts, at
the commit named by `DECK_TASK_COMMIT` (default `HEAD`):

1. `git log -1 --format=%H <commit> -- '*.go' '*.feature'` == REV — the pin is
   still the last code-touching commit;
2. one parametrized test per path (`internal`, `cmd`, `features`, `ci`):
   `rev-parse REV:<path>` == `rev-parse <commit>:<path>`, and equals the
   hash tabled above — the four pairs;
3. `git diff --name-only REV <commit>` lists no `.go`/`.feature` path
   (corroboration);
4. one parametrized test per cure commit: `git cat-file -e <sha>^{commit}`
   exits 0 — the four commits of tasks 001-004.

Ten tests, green both before and at this task's own publishing commit:

| run | `DECK_TASK_COMMIT` | result | log |
| --- | --- | --- | --- |
| before this task's commit | `485f9b547a096895c609211e4936aeda888b3b9d` | 10 passed, pytest exit 0 | `docs/reports/phase4b-a2-code-revision/pytest-pre-commit.log` |
| at this task's publishing commit | `154c63e109e4cfebd9586eae8c5328eeee823930` | 10 passed, pytest exit 0 | `docs/reports/phase4b-a2-code-revision/pytest-post-commit.log` |

Re-run it (the harness file is suffixed `.py.txt` in the repo precisely so
that it can never be collected as a test of this repo — the deck repo's test
surface stays Go test functions and godog scenarios only, and no test
framework is added to it):

```
mkdir -p /tmp/verify005
cp docs/reports/phase4b-a2-code-revision/pytest-tree-hash-pairs.py.txt \
   /tmp/verify005/test_pinned_revision.py
python3 -m pytest --version                 # pip install --user pytest, if absent
DECK_REPO=/workspace python3 -m pytest -v -p no:cacheprovider \
   /tmp/verify005/test_pinned_revision.py
```

With `DECK_TASK_COMMIT` left unset it reads `HEAD`, so the run stays green at
this task's own commits and at every later record-only commit of the freeze —
a reader can re-verify all four pairs at whatever commit they have checked
out, without editing anything.

## The four cure commits this revision is built from

| task | finding | commit | subject |
| --- | --- | --- | --- |
| 001 | B1 | `113b552ca2fe42bb1b3d10e42f14cdc3f735c8e6` | tui: match the default group's label in the list filter (B1, #25) |
| 002 | B2 | `075c59c35fbbc22461f748bbe87e87a8ac4139e4` | tui: refresh an open settings Groups panel on the ordinary reload (B2) |
| 003 | B3 | `00f33a5ac204bc2f558352a18c40fefa24ad3bab` | interactive: clear the scrollback the displacement notice itself creates (B3, #30) |
| 004 | R1 | `70c7430df3b23a46fb735e8573e26ec55908adeb` | tui: name the manual group model in the c help line, not the removed workspace model (R1) |

Each resolves as a commit object (`git cat-file -e <sha>^{commit}`, exit
status 0 for all four), re-derived at this task's own commit. Committed
output: `docs/reports/phase4b-a2-code-revision/cure-commits-resolve.log`;
the same four checks also run as pytest cases in the runs tabled above.

## For downstream tasks

Tasks 006-012 cite the four tree-object hashes above (labelled `internal`,
`cmd`, `features`, `ci`) as the identity of the tree they measured/attested
against. Any measurement whose own `git rev-parse <rev>:<path>` does not
match one of these four hashes is measuring a different tree than this
approach pinned and must not be cited as approach-2 evidence.
