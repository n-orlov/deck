# Task 207 — protected-path audit re-run at the final sha

## Command

BASE was computed, not pasted:

```
BASE=$(git log --format=%H --diff-filter=A -1 -- prds/phase3k-agent-availability.md)
HEAD=$(git rev-parse HEAD)
git log --oneline "$BASE..HEAD" -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
```

Full transcript in `audit.log`:

```
BASE=150d7d6f26c9fa47648214dc1a446c36ff23a376
HEAD=04a7e261f55814637393deba05fd0ded2266ce58
EXIT=0
```

(`git log --oneline "$BASE..HEAD" -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md` produced no
output between the echoed `HEAD=` line and the echoed `EXIT=0` line.)

## Disposition

**Clean — zero commits touch a protected path between BASE and HEAD.** The audit
command printed zero lines over the range `150d7d6f26c9fa47648214dc1a446c36ff23a376..04a7e26
1f55814637393deba05fd0ded2266ce58` for `SPEC.md`, `prds/`, `ci/Dockerfile`, and
`ci/SPIKE.md`. No protected-path edit occurred anywhere in this approach's history
(tasks 201–206), consistent with the standing rule that those paths are read-only for
this run. No finding to record in `docs/reports/phase3k-findings.md` from this audit.

BASE is the commit that added `prds/phase3k-agent-availability.md`
(`150d7d6f26c9fa47648214dc1a446c36ff23a376`); HEAD is the current tip of `main` at the
time of this audit (`04a7e261f55814637393deba05fd0ded2266ce58`, task 206's commit). The
final code sha (last commit touching `*.go`/`*.feature`) remains
`4e09f2de90dcde04bd8fc20c77097e593f2fee5b`; HEAD is a docs-only descendant of it, which
this audit does not need to distinguish since it inspects the whole BASE..HEAD range for
protected-path touches, not just the code-sha boundary.
