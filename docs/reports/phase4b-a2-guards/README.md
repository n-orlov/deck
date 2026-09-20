# Phase 4b, approach 2 — task 012: build/vet/gofmt + protected-path guards

## Sha this task's checks ran at

This report's own commit lands on top of **28859af795ef01cdf5b686e748a0e6e48b91a411**
(task 011, the last commit before this one) — a record-only tree (freeze has held since
task 004's `70c7430`), so the `go build`/`go vet`/`gofmt` guards below measure the same
code tree the whole approach has been running against throughout tasks 005-011.

## `ci/run.sh go build ./...` and `ci/run.sh go vet ./...`

Both exit 0. Raw logs (both empty — clean build/vet produce no output):

- `build.log` — `go build ./...`, exit 0
- `vet.log` — `go vet ./...`, exit 0

## `ci/run.sh gofmt -l .`

Exit 0. Output (`gofmt.log`) lists exactly the three known pre-existing
`.spike-preview/` files, nothing else:

```
.spike-preview/cmd/conformance/main.go
.spike-preview/conformance/conformance.go
.spike-preview/conformance/conformance_test.go
```

`internal/theme/quantize_test.go` is not listed — confirmed clean, consistent with
task 010's finding that the PRD's claim about it is stale.

## Protected read-only paths — HEAD vs the approach-2 launch sha (5128783)

Approach 2's launch sha is `5128783bb39bac130ec1bab5e53338f3bbb951ac` (per the notes'
standing rules). For each of the four read-only paths, the object hash at HEAD
(`28859af795ef01cdf5b686e748a0e6e48b91a411`) and at `5128783` are shown side by side
(`git rev-parse <rev>:<path>`; full log in `protected-hashes.log`):

| path            | HEAD hash                                 | 5128783 hash                               | equal |
|-----------------|--------------------------------------------|---------------------------------------------|-------|
| `SPEC.md`       | `988a90f7f7aa8e2f5d1b99af9e0a77549230091d` | `988a90f7f7aa8e2f5d1b99af9e0a77549230091d`  | YES   |
| `prds`          | `59f7547bf504aa398778c4b68de11a32cbe24cfb` | `59f7547bf504aa398778c4b68de11a32cbe24cfb`  | YES   |
| `ci/Dockerfile` | `06fdfbd947b9cc4289fd28c0a757e858d2cc8018` | `06fdfbd947b9cc4289fd28c0a757e858d2cc8018`  | YES   |
| `ci/SPIKE.md`   | `b57b7e96b00937d973d6ead5c08d36ff74923ab8` | `b57b7e96b00937d973d6ead5c08d36ff74923ab8`  | YES   |

All four pairs equal — none of these paths were touched anywhere in approach 2 (001-011),
consistent with the standing rule that they are read-only for the whole run.

## HEAD vs origin/main after this task's own push

This task's own commit had to exist and be pushed before this comparison could be taken
(a commit cannot quote its own not-yet-existing sha). See `post-push-heads.log`, added in
the follow-up record-only commit immediately after this one pushes: it re-runs
`git rev-parse HEAD` and `git rev-parse origin/main` and shows them equal, closing the
approach's own final clean boundary.

## Files in this directory

- `build.log`, `vet.log`, `gofmt.log` — raw guard output (build/vet empty on success)
- `protected-hashes.log` — raw `git rev-parse` output for the four protected-path pairs
- `post-push-heads.log` — added by the follow-up commit; HEAD == origin/main after push
