# Phase 3j task 033 — protected-path audit and branch guards re-verify

Re-verified at the true final code sha, in a single iteration (2026-09-03), per
task 033's `successCriteria`. This README supersedes the first revision of this
same directory, which rendered the two zero-output commands as the literal word
`(empty)` (a summary) and displayed an audit invocation that was not the one
run; that revision is history, not evidence. Nothing here is summarised: every
byte below is the exact command output, and the two commands that print nothing
are shown as nothing, with their captured byte counts as corroboration.

All four commands were run back-to-back in one iteration, on a clean tree, at
`HEAD` = `origin/main` = `fe18ea973592ab5b8683d37b9ed5b607ca76f3fe` (the commit
publishing this README is a docs-only descendant of it and does not move the
final code sha).

## Single-session transcript (verbatim)

`transcript.log` in this directory is the capture, reproduced byte-for-byte
below. It was produced by `capture.sh` (also in this directory), whose `run()`
helper prints the prompt plus the command string and then `eval`s that same
string, so each displayed command line is byte-for-byte the command executed —
no command, echo or annotation is inserted between, inside or around them. Each
command's stdout and stderr go straight into the transcript; the next `$ `
prompt line marks the end of the previous command's output, so a command that
printed zero bytes shows as nothing between its command line and the next
prompt.

```
$ git log -1 --format=%H -- '*.go' '*.feature'
a44ee320b93186496d56364836b0aed00a6f1e0b
$ BASE=$(git log --format=%H --diff-filter=A -1 -- prds/phase3j-launch-and-teardown-hooks.md); git log --oneline "$BASE..HEAD" -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
$ git status --porcelain
$ git rev-parse HEAD origin/main
fe18ea973592ab5b8683d37b9ed5b607ca76f3fe
fe18ea973592ab5b8683d37b9ed5b607ca76f3fe
$ 
```

## Per-command captures

The same four commands were also captured one per file in the same iteration
(stdout and stderr both redirected). Each file is committed in this directory
alongside this README; the fenced block under each command holds that file's
exact contents.

### 1. Recomputed final code sha — `final-code-sha.out`

```
git log -1 --format=%H -- '*.go' '*.feature'
```

```
a44ee320b93186496d56364836b0aed00a6f1e0b
```

### 2. Protected-path audit, run verbatim — `protected-path-audit.out`

The command is the standing-rules/PRD audit, copied verbatim and run as a
single command line with nothing inserted into it:

```
BASE=$(git log --format=%H --diff-filter=A -1 -- prds/phase3j-launch-and-teardown-hooks.md); git log --oneline "$BASE..HEAD" -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
```

Its exact output, which is zero bytes — the fenced block below is empty because
the command printed nothing at all:

```
```

`protected-path-audit.out` is a 0-byte file (see the byte counts below), so no
commit between the PRD's own base commit and `HEAD` touches `SPEC.md`, `prds/`,
`ci/Dockerfile` or `ci/SPIKE.md`. The guard holds.

As a separate command, run after the verbatim audit above and not part of it,
the base commit the audit's `$BASE` resolves to:

```
git log --format=%H --diff-filter=A -1 -- prds/phase3j-launch-and-teardown-hooks.md
```

```
06ea5b72d98ce0dda251d16af452b7f6d87e2c2f
```

### 3. Working tree cleanliness — `git-status-porcelain.out`

```
git status --porcelain
```

Its exact output, which is zero bytes — the fenced block below is empty because
the command printed nothing at all:

```
```

`git-status-porcelain.out` is a 0-byte file (see the byte counts below): the
tree is clean, with no modified, staged or untracked path.

### 4. Branch guard — `rev-parse-head-origin-main.out`

```
git rev-parse HEAD origin/main
```

```
fe18ea973592ab5b8683d37b9ed5b607ca76f3fe
fe18ea973592ab5b8683d37b9ed5b607ca76f3fe
```

The two lines are identical, so `HEAD` and `origin/main` are the same commit:
the tree is fully pushed and in sync with the remote's `main`.

### Byte counts of the four capture files

```
wc -c final-code-sha.out protected-path-audit.out git-status-porcelain.out rev-parse-head-origin-main.out
```

```
 41 final-code-sha.out
  0 protected-path-audit.out
  0 git-status-porcelain.out
 82 rev-parse-head-origin-main.out
123 total
```

The two 0-byte files are the audit and `git status --porcelain`: their exact
output is the empty string.
