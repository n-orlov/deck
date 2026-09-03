#!/bin/sh
# Faithful transcript driver for task 033's four guard commands.
#
# run() prints the prompt followed by the command string EXACTLY as given — a
# multi-line command keeps its newlines, nothing is joined onto one line, and
# no newline is replaced by "; " — then eval's that same string, so the
# displayed command text is byte-for-byte the command executed. Nothing is
# inserted between, inside or around the commands. Each command's stdout and
# stderr land in the transcript as emitted, and the next "$ " prompt line marks
# the end of the previous command's output, so a command printing zero bytes
# shows as nothing between its command text and the next prompt.
#
# The protected-path audit is held in $AUDIT exactly as the PRD's code block
# writes it: two lines separated by a newline, the BASE assignment first and
# the `git log` second.
#
# Every file this driver writes goes into an output directory OUTSIDE the git
# work tree (default /tmp/phase3j-033-capture), so the measurement cannot
# perturb the `git status --porcelain` it measures. The captures are copied
# into docs/reports/phase3j-033-guards/ afterwards.
#
# Usage, from the repository root:
#   sh docs/reports/phase3j-033-guards/capture.sh [output-dir]

set -u

OUT=${1:-/tmp/phase3j-033-capture}
mkdir -p "$OUT"

SHA_CMD="git log -1 --format=%H -- '*.go' '*.feature'"
AUDIT='BASE=$(git log --format=%H --diff-filter=A -1 -- prds/phase3j-launch-and-teardown-hooks.md)
git log --oneline "$BASE..HEAD" -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md'
STATUS_CMD='git status --porcelain'
REVPARSE_CMD='git rev-parse HEAD origin/main'

run() {
  printf '$ %s\n' "$1"
  eval "$1" 2>&1
}

# Phase 1: one single-session transcript of the four commands, in order.
{
  run "$SHA_CMD"
  run "$AUDIT"
  run "$STATUS_CMD"
  run "$REVPARSE_CMD"
  printf '$ \n'
} > "$OUT/transcript.log" 2>&1

# Phase 2: the same four commands again, one capture file each (stdout+stderr).
eval "$SHA_CMD"      > "$OUT/final-code-sha.out"             2>&1
eval "$AUDIT"        > "$OUT/protected-path-audit.out"       2>&1
eval "$STATUS_CMD"   > "$OUT/git-status-porcelain.out"       2>&1
eval "$REVPARSE_CMD" > "$OUT/rev-parse-head-origin-main.out" 2>&1

# Phase 3: byte counts of the four capture files, corroborating the two that
# are legitimately empty.
( cd "$OUT" && wc -c final-code-sha.out protected-path-audit.out \
    git-status-porcelain.out rev-parse-head-origin-main.out ) > "$OUT/byte-counts.out" 2>&1
