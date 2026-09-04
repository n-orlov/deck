# Phase 3j task 033 — protected-path audit and branch guards re-verify

Re-verified at the true final code sha, in a single iteration (2026-09-04), per
task 061's `successCriteria`. This README refreshes this same directory in
place, superseding its earlier revisions:

- revision 1 rendered the two zero-output commands as the literal word
  `(empty)` (a summary, not exact output) and displayed an audit invocation
  that was not the one run;
- revision 2 pasted exact output, but joined the PRD audit's two lines with
  `; ` and executed that semicolon-joined string, so the audit was not run
  verbatim as the PRD writes it;
- revision 3 fixed both defects and ran the audit correctly, but its code sha
  (`a44ee320b93186496d56364836b0aed00a6f1e0b`) was superseded once tasks 057–060
  landed further docs-only and code commits, moving the final code sha to
  `b29afb8c4fd8a1cf193c7efef5c5f7e1456481f7`;
- revision 4 re-ran the same driver at that sha and published a fresh set of
  captures, but was itself superseded once task 080's comment-only
  `features/launch_hooks.feature` edit moved the final code sha again, per the
  PRD's Termination rule, to `4fbd452430501805a860dd229ddca1cd3f5c1cd6`.

All four are history, not evidence. This revision re-runs the same
`capture.sh` driver — unchanged since revision 3 — at the current tree, where
the final code sha is now task 080's own commit,
`4fbd452430501805a860dd229ddca1cd3f5c1cd6`: the same sha cited by the refreshed
READMEs of `docs/reports/phase3j-030-fullsuite`,
`docs/reports/phase3j-031-fullsuite-verbose` and
`docs/reports/phase3j-032-stability10`.

All four commands were run back-to-back in one iteration, on a clean tree, at
`HEAD` = `origin/main` = `7f5762480a03fb49b9a06ce2bf98b1d16b450083` (this
README's own publishing commit is a docs-only descendant of it and does not
move the final code sha, which stays `4fbd452430501805a860dd229ddca1cd3f5c1cd6`).

## Single-session transcript (verbatim)

`transcript.log` in this directory is the capture, reproduced byte-for-byte
below. It was produced by `capture.sh` (also in this directory), whose `run()`
helper prints the prompt plus the command string and then `eval`s that same
string, so each displayed command is byte-for-byte the command executed —
multi-line commands keep their newlines, nothing is joined onto one line, and no
newline is replaced by `; `. No command, echo or annotation is inserted between,
inside or around them. Each command's stdout and stderr go straight into the
transcript; the next `$ ` prompt line marks the end of the previous command's
output, so a command that printed zero bytes shows as nothing between its
command text and the next prompt.

`capture.sh` writes every file it produces into a directory outside the git work
tree (default `/tmp/phase3j-033-capture`), and the captures were copied into this
directory afterwards, so the measurement itself could not perturb the
`git status --porcelain` it measures — which is why the transcript's status
output below is genuinely empty on a tracked-and-clean tree.

```
$ git log -1 --format=%H -- '*.go' '*.feature'
4fbd452430501805a860dd229ddca1cd3f5c1cd6
$ BASE=$(git log --format=%H --diff-filter=A -1 -- prds/phase3j-launch-and-teardown-hooks.md)
git log --oneline "$BASE..HEAD" -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
$ git status --porcelain
$ git rev-parse HEAD origin/main
7f5762480a03fb49b9a06ce2bf98b1d16b450083
7f5762480a03fb49b9a06ce2bf98b1d16b450083
$ 
```

Note the audit's two lines in that transcript: the prompt precedes the `BASE=`
assignment line, and the `git log` line follows it on its own line, exactly as
the PRD's code block has it.

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
4fbd452430501805a860dd229ddca1cd3f5c1cd6
```

### 2. Protected-path audit, run verbatim — `protected-path-audit.out`

The command is the PRD's audit, copied verbatim from
`prds/phase3j-launch-and-teardown-hooks.md` and run as the two newline-separated
lines it is, with nothing inserted into or between them:

```
BASE=$(git log --format=%H --diff-filter=A -1 -- prds/phase3j-launch-and-teardown-hooks.md)
git log --oneline "$BASE..HEAD" -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
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
7f5762480a03fb49b9a06ce2bf98b1d16b450083
7f5762480a03fb49b9a06ce2bf98b1d16b450083
```

The two lines are identical, so `HEAD` and `origin/main` are the same commit:
the tree is fully pushed and in sync with the remote's `main`.

### Byte counts of the four capture files — `byte-counts.out`

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
