# Phase 4 review protocol — the Go analogue of the reviewer's disposable-clone check (B0)

## What review's own template checks, and why it does not apply here as written

Review's template measurement protocol installs a package into a fresh, disposable
clone (or venv) and asserts the imported package's resolved location (Python's
`__file__`) lies under that clone, not under the original checkout — proof that the
artifact actually under test is the clone's own copy, not something picked up from a
stale install, a cached `site-packages` entry, or the original tree via some
implicit search-path fallback.

**This repository's product is the Go module `github.com/n-orlov/deck`. It contains
no Python package, no `setup.py`, no `pyproject.toml`, and no importable Python
module of any kind** — there is nothing for the reviewer's own Python check to import,
and adding Python packaging to this repository solely to make that template's exact
commands run would be introducing product surface that does not exist for a check
review itself does not require of non-Python repositories. That is not a gap this
report leaves open; it is why this document exists instead: the same *property* the
Python check verifies (the identity of the code under test) has a direct Go
equivalent, and `ci/review.sh` (tracked, executable, at the repo root's `ci/`
directory) implements it.

## The Go analogue, concretely

`ci/review.sh <go test args...>`:

1. Makes a disposable, git-ignored clone of this repository at
   `<repo-root>/.review-clone` (`git clone --local --no-hardlinks`, removed and
   recreated on every invocation — see "Git-ignored" below).
2. Inside that clone, through the existing `ci/run.sh` sibling container
   (`deck-ci:local`, the same toolchain every other task in this run uses), asserts
   the **identity check**:
   - `go list -m` resolves the module path to exactly `github.com/n-orlov/deck`.
   - `go list -f '{{.Dir}}' ./internal/agent` resolves to a directory under the
     clone's own tree (`/w/.review-clone/...` — `/w` is `ci/run.sh`'s fixed mount
     point for the repository root inside the sibling), and specifically **not** to
     `/w/internal/agent`, the original checkout's own copy. A match on the clone's
     prefix combined with a mismatch against the original path is the Go equivalent
     of the reviewer's "imported location is under the clone, not the original
     checkout" assertion — it proves the package Go is about to build and test is
     the clone's own copy, not the tree this job is editing.
3. Only once that identity assertion passes does it run the caller-supplied
   `go test` target, verbatim, inside the **same** clone, through the same sibling.
   The script does not choose or narrow that target; the caller does.

## The three invocations a reviewer would use

- **Identity assertion alone** — implicit in every invocation below; it always runs
  first and the script exits non-zero before any test runs if it fails.
- **Narrow smoke** (this task's own executed evidence — never a whole-suite run):

  ```console
  $ ci/review.sh ./internal/agent/
  review.sh: cloned /workspace -> /workspace/.review-clone
  review.sh: identity ok -- module=github.com/n-orlov/deck dir=/w/.review-clone/internal/agent
  review.sh: running go test ./internal/agent/ inside the clone
  ok  	github.com/n-orlov/deck/internal/agent	0.008s
  ```

  (Full transcript, same output, is this task's own evidence and is not re-run by any
  later task per the standing rules' Materiality termination rule.)

- **Full-gate target** — the same shape task 013 uses for the run's actual full-suite
  gate sweep, available to a reviewer who wants the whole tree exercised inside the
  disposable clone rather than in place:

  ```console
  $ ci/review.sh -p=1 -count=1 -timeout=40m ./...
  ```

  This report does not execute that invocation — task 013 is the one whole-tree
  sweep of record for this approach, run once, at the tail code sha, in place. This
  document only names the target shape the sweep would take if run through
  `ci/review.sh` instead.

## Read-only paths

`ci/review.sh` neither reads from nor writes to any of the following; they are
read-only for this job's whole run, per the standing rules, and this task does not
touch them:

- `SPEC.md`
- `prds/`
- `ci/Dockerfile`
- `ci/SPIKE.md`

## Git-ignored clone directory

`.review-clone/` (rooted at the repository root, matching `ci/review.sh`'s own
`clone_dir`) is listed in `.gitignore` (`/.review-clone/`). Nothing under it is ever
tracked; each `ci/review.sh` invocation removes and recreates it from scratch.

## Why this is not curable by adding Python packaging

Review's B0 finding names a specific measurement template written against Python's
import machinery. This repository has no Python surface for that template to check,
and the standing rules for this approach say so explicitly: adding `setup.py`,
`pyproject.toml`, a `ralphd` module, or any other Python packaging to this Go
repository, solely to make a Python-shaped check runnable, is not a cure — it adds
product surface review never asked for, to satisfy a template rather than the
property the template exists to verify. `ci/review.sh` verifies that property (clone
identity, not original-checkout identity) using the tools this module actually has:
`git clone`, `go list -m`, and `go list -f '{{.Dir}}'`.
