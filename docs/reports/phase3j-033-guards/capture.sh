#!/bin/sh
# Faithful transcript driver: the SAME string is printed after the prompt and
# then eval'd, so the displayed command line is byte-for-byte the command run.
# Nothing is inserted between or inside the commands; each command's stdout and
# stderr land in the transcript exactly as emitted, and a trailing prompt line
# marks the end of its output (so zero-byte output shows as nothing between the
# command line and the next prompt).
run() {
  printf '$ %s\n' "$1"
  eval "$1" 2>&1
}
run "git log -1 --format=%H -- '*.go' '*.feature'"
run 'BASE=$(git log --format=%H --diff-filter=A -1 -- prds/phase3j-launch-and-teardown-hooks.md); git log --oneline "$BASE..HEAD" -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md'
run 'git status --porcelain'
run 'git rev-parse HEAD origin/main'
printf '$ \n'
