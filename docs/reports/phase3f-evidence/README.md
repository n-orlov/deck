# Phase 3f per-task evidence (verbatim copies of the run's artifacts directory)

Every file in this directory is a **byte-identical copy** of a file the phase's
own tasks wrote into the autonomous run's artifacts directory
(`/run/ralphd/artifacts/…`) as they landed. They are committed here so that
[`../phase3f.md`](../phase3f.md)'s citations resolve for a reader who only has
the repository — the run directory is not part of the repo and does not outlive
the run.

- Filenames keep their `taskNNN-` prefix, which is the phase's task number, not
  a requirement number. The mapping from requirement to task is the table in
  [`../phase3f.md`](../phase3f.md).
- `.md` files are the per-task write-ups (what changed, what the test pins, the
  revert-and-reproduce transcript). `.log`/`.txt` files are raw command output.
- Godog failure logs contain a full SIGQUIT goroutine dump, so some are large
  and have single lines hundreds of kilobytes long. Read them with
  `awk 'length($0)<400' <file>` rather than grepping with context.
- Files that a task already committed under its own report directory
  (`phase3f-016-…md`, `phase3f-019-…`, `phase3f-020-…`, `phase3f-022-stability10/`)
  were **not** duplicated here; cite those paths directly.
- `planner-baseline-suite.log` is the planning iteration's baseline whole-suite
  run at `60c2c56`, kept for the before/after wall-clock comparison; it is not
  evidence for any requirement.
- `check-citations.sh` is the one file here that is **not** a copy of an artifact:
  it is the checker task 023 ran over [`../phase3f.md`](../phase3f.md), asserting
  that every sha it cites resolves (`git cat-file -e`) and every relative path it
  links to exists. Run it from the repository root; it exits non-zero on the first
  dangling citation.
