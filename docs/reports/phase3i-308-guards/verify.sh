#!/bin/sh
# Phase 3i task 308 — a re-runnable guard that re-verifies every citation the
# approach-3 documents make, at whatever HEAD it is invoked on.
#
# Pure POSIX sh + git + awk (no Go, no python, no network). Run from anywhere inside
# the repo:
#
#   sh docs/reports/phase3i-308-guards/verify.sh
#
# It prints one labelled PASS line for each of five independent checks and exits 0
# only if all five held. Any failure prints one or more FAIL lines naming exactly
# what broke and the script exits 1.
#
# The three documents in scope are docs/reports/phase3i.md, docs/reports/
# phase3i-findings.md (both in full) and docs/DELIVERY-LOG.md's Phase 3i paragraph
# ONLY (extracted below by its own **Phase 3i** marker up to the next "## " heading,
# so the other 20-odd phases DELIVERY-LOG.md also narrates are never scanned).

set -u
cd "$(git rev-parse --show-toplevel)" || exit 1
rc=0
fail() { printf 'FAIL: %s\n' "$1"; rc=1; }
pass() { printf 'PASS: %s\n' "$1"; }

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

awk '/^## Other milestones/{exit} /^\*\*Phase 3i\*\*/{flag=1} flag{print}' \
  docs/DELIVERY-LOG.md > "$tmpdir/delivery-log-phase3i.md"

if [ ! -s "$tmpdir/delivery-log-phase3i.md" ]; then
  fail "could not extract a non-empty Phase 3i paragraph from docs/DELIVERY-LOG.md"
fi

DOCS="docs/reports/phase3i.md docs/reports/phase3i-findings.md $tmpdir/delivery-log-phase3i.md"

# --- check 1: every sha cited in the three documents resolves ---------------------
n=0
c1=0
for s in $(grep -oh '`[0-9a-f]\{7,40\}`' $DOCS | tr -d '`' | sort -u); do
  n=$((n + 1))
  if ! git cat-file -e "${s}^{commit}" 2>/dev/null; then
    fail "cited sha $s does not resolve (git cat-file -e)"
    c1=1
  fi
done
[ "$c1" -eq 0 ] && pass "check 1: all $n distinct cited shas resolve (git cat-file -e)"

# --- check 2: every path cited in the three documents is tracked ------------------
# A handful of backtick tokens are excluded by name or shape: `tasks.json` and any
# `/run/ralphd/...` path are the run's OWN state files, which live outside this repo
# by design and are quoted verbatim from prose describing the run, not asserted as a
# repo path; a bare `/` is markdown punctuation between two other backtick tokens
# (e.g. `*.go`/`*.feature`), not a path at all. Ordinary shorthand basenames like
# `ownership.go` or `tui.go` are NOT excluded — they genuinely resolve via the
# basename fallback below, against their one real tracked file.
m=0
c2=0
resolves_as_file() {
  git ls-files --error-unmatch "$1" >/dev/null 2>&1
}
resolves_as_dir() {
  [ -n "$(git ls-files -- "$1")" ]
}
check_one_path() {
  raw="$1"
  p="$raw"
  case "$p" in */) p="${p%/}" ;; esac
  if resolves_as_file "$p" || resolves_as_dir "$p"; then return 0; fi
  if resolves_as_file "docs/reports/$p" || resolves_as_dir "docs/reports/$p"; then return 0; fi
  if resolves_as_file "docs/$p" || resolves_as_dir "docs/$p"; then return 0; fi
  if [ "$(git ls-files | grep -c "/${p}\$")" -ge 1 ]; then return 0; fi
  return 1
}

for raw in $(grep -oh '`[^` ]*`' $DOCS | tr -d '`' | sed 's/:[0-9]*-\{0,1\}[0-9]*$//' | sort -u); do
  case "$raw" in
    tasks.json) continue ;;
    ''|/) continue ;;
    '~'*|'#'*|/run/ralphd/*) continue ;;
  esac
  case "$raw" in
    *.md|*.go|*.feature|*.sh|*.toml|*.log|*.exitstatus|*/) ;;
    */*) ;;
    *) continue ;;
  esac
  m=$((m + 1))
  if ! check_one_path "$raw"; then
    fail "cited path $raw is not tracked (git ls-files --error-unmatch)"
    c2=1
  fi
done

for l in $(grep -oh '](reports/[^)) ]*)\|](phase3i[^)) ]*)' $DOCS | sed 's/^](//; s/)$//' | sort -u); do
  case "$l" in '#'*) continue ;; esac
  t="${l%%#*}"
  m=$((m + 1))
  if ! check_one_path "$t"; then
    fail "link target $l is not tracked (git ls-files --error-unmatch)"
    c2=1
  fi
done
[ "$c2" -eq 0 ] && pass "check 2: all $m distinct cited paths pass git ls-files --error-unmatch"

# --- check 3: worktree is clean ---------------------------------------------------
status=$(git status --porcelain)
if [ -n "$status" ]; then
  printf '%s\n' "$status"
  fail "worktree not clean (git status --porcelain non-empty)"
else
  pass "check 3: git status --porcelain is empty"
fi

# --- check 4: HEAD is pushed -------------------------------------------------------
h=$(git rev-parse HEAD)
o=$(git rev-parse origin/main 2>/dev/null || echo "")
if [ "$h" = "$o" ] && [ -n "$h" ]; then
  pass "check 4: git rev-parse HEAD origin/main agree ($h)"
else
  fail "HEAD ($h) != origin/main ($o)"
fi

# --- check 5: no worker commit touched a protected path ---------------------------
LASTPROT=3090b68
prot=$(git log --oneline "$LASTPROT..HEAD" -- SPEC.md prds ci/Dockerfile ci/SPIKE.md)
if [ -n "$prot" ]; then
  printf '%s\n' "$prot"
  fail "a worker commit touched a protected path since $LASTPROT"
else
  pass "check 5: git log --oneline $LASTPROT..HEAD -- SPEC.md prds ci/Dockerfile ci/SPIKE.md is empty"
fi

if [ "$rc" -eq 0 ]; then
  echo "ALL GUARDS OK at $(git rev-parse HEAD)"
else
  echo "GUARD FAILURES at $(git rev-parse HEAD)"
fi
exit "$rc"
