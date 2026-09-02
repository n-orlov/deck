#!/bin/sh
# Phase 3i task 134 — re-run every guard this report publishes, at whatever HEAD it is
# invoked on. Pure POSIX sh + git (no Go, no python, no network): a reader can run
#
#   sh docs/reports/phase3i-134-guards/verify.sh
#
# from the repo root and compare its output with README.md's quoted blocks. Exit 0 means
# every assertion below held at the HEAD it ran against; any failure prints FAIL and the
# script exits 1.
#
# The one guard that is expected NOT to be empty is the standing rules' literal
# protected-path range `6197b53..HEAD`: it contains exactly one commit, the operator's
# own PRD-cut commit 3090b68, established in docs/reports/phase3i-findings.md §2. This
# script asserts that commit is the ONLY entry in the range and that the worker-write
# range 3090b68..HEAD is empty.

set -u
cd "$(git rev-parse --show-toplevel)" || exit 1
rc=0
fail() { printf 'FAIL: %s\n' "$1"; rc=1; }

DOCS='docs/reports/phase3i.md docs/reports/phase3i-findings.md'
BASE=6197b53
LASTPROT=3090b68

echo "=== git status --porcelain"
status=$(git status --porcelain)
if [ -n "$status" ]; then printf '%s\n' "$status"; fail "worktree not clean"; else echo "(empty)"; fi

echo "=== git rev-parse HEAD origin/main"
git rev-parse HEAD origin/main
h=$(git rev-parse HEAD); o=$(git rev-parse origin/main)
[ "$h" = "$o" ] || fail "HEAD ($h) != origin/main ($o)"

echo "=== git log --oneline $BASE..HEAD -- SPEC.md prds ci/Dockerfile ci/SPIKE.md (literal standing-rule range)"
lit=$(git log --oneline "$BASE..HEAD" -- SPEC.md prds ci/Dockerfile ci/SPIKE.md)
if [ -n "$lit" ]; then printf '%s\n' "$lit"; else echo "(empty)"; fi
litshas=$(git log --format=%h "$BASE..HEAD" -- SPEC.md prds ci/Dockerfile ci/SPIKE.md)
if [ "$litshas" != "$(git rev-parse --short=7 $LASTPROT)" ]; then
  fail "literal range contains something other than the operator commit $LASTPROT alone"
fi

echo "=== git log --oneline $LASTPROT..HEAD -- SPEC.md prds ci/Dockerfile ci/SPIKE.md (worker-write range)"
work=$(git log --oneline "$LASTPROT..HEAD" -- SPEC.md prds ci/Dockerfile ci/SPIKE.md)
if [ -n "$work" ]; then printf '%s\n' "$work"; fail "a worker commit touched a protected path"; else echo "(empty)"; fi

echo "=== git diff --stat $LASTPROT..HEAD -- SPEC.md prds ci/Dockerfile ci/SPIKE.md"
d=$(git diff --stat "$LASTPROT..HEAD" -- SPEC.md prds ci/Dockerfile ci/SPIKE.md)
if [ -n "$d" ]; then printf '%s\n' "$d"; fail "protected paths differ from $LASTPROT"; else echo "(empty)"; fi

echo "=== every sha cited in phase3i.md + phase3i-findings.md resolves (git cat-file -e)"
n=0
for s in $(grep -oh '`[0-9a-f]\{7,40\}`' $DOCS | tr -d '`' | sort -u); do
  if git cat-file -e "${s}^{commit}" 2>/dev/null; then echo "$s ok"; else echo "$s MISSING"; fail "cited sha $s does not resolve"; fi
  n=$((n+1))
done
echo "($n distinct cited shas)"

echo "=== every path cited in phase3i.md + phase3i-findings.md is tracked (git ls-files --error-unmatch)"
# Two backtick tokens are excluded by name: they appear ONLY inside a verbatim quotation
# of task 119's own validationNotes text in phase3i-findings.md §1, so they are not path
# assertions in the report's own voice. `ownership.go` is that quotation's shorthand for
# internal/tmux/ownership.go (checked below in its full form); `tasks.json` is the run
# state file, which lives outside the repo by design.
m=0
for raw in $(grep -oh '`[^` ]*`' $DOCS | tr -d '`' | sed 's/:[0-9]*-[0-9]*$//' | sort -u); do
  case "$raw" in
    ownership.go|tasks.json) continue ;;
    '~'*|'#'*) continue ;;
  esac
  case "$raw" in
    *.md|*.go|*.feature|*.sh|*.toml|*.log|*.exitstatus|*/) ;;
    */*) ;;
    *) continue ;;
  esac
  p="$raw"
  case "$p" in */) p="${p%/}" ;; esac
  if git ls-files --error-unmatch "$p" >/dev/null 2>&1; then echo "$raw ok (file)"
  elif [ -n "$(git ls-files -- "$p")" ]; then echo "$raw ok (tracked directory)"
  elif git ls-files --error-unmatch "docs/reports/$p" >/dev/null 2>&1; then echo "$raw ok (file, relative to docs/reports/)"
  elif [ -n "$(git ls-files -- "docs/reports/$p")" ]; then echo "$raw ok (tracked directory, relative to docs/reports/)"
  elif [ "$(git ls-files | grep -c "/${p}$")" -ge 1 ]; then
    # A bare basename used as a markdown LINK LABEL (e.g. `sweep.log.exitstatus`), whose
    # link target is the full relative path checked in the link section below.
    echo "$raw ok (link label; tracked as $(git ls-files | grep "/${p}$" | tr '\n' ' '))"
  else echo "$raw UNTRACKED"; fail "cited path $raw is not tracked"; fi
  m=$((m+1))
done
echo "($m distinct cited paths)"

echo "=== markdown link targets in both documents"
for l in $(grep -oh ']([^)) ]*)' $DOCS | sed 's/^](//; s/)$//' | sort -u); do
  case "$l" in '#'*) echo "$l ok (in-document anchor)"; continue ;; esac
  t="${l%%#*}"
  if git ls-files --error-unmatch "docs/reports/$t" >/dev/null 2>&1; then echo "$l ok"
  elif git ls-files --error-unmatch "$t" >/dev/null 2>&1; then echo "$l ok"
  else echo "$l UNTRACKED"; fail "link target $l is not tracked"; fi
done

echo "=== final code sha (git log -1 --format=%H -- '*.go' '*.feature')"
git log -1 --format=%H -- '*.go' '*.feature'

if [ "$rc" -eq 0 ]; then echo "ALL GUARDS OK at $(git rev-parse HEAD)"; else echo "GUARD FAILURES at $(git rev-parse HEAD)"; fi
exit "$rc"
