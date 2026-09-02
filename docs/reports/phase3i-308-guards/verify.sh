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
# The scan is deliberately WIDER than "backtick-quoted token": EVERY maximal run of 7-40
# lowercase hex characters anywhere in the three documents is treated as a cited sha and
# must resolve. That is what makes all three citation shapes these documents actually use
# checkable, rather than only the first:
#   * a sha quoted on its own                 -- `a559e7c`
#   * either endpoint of a quoted commit range -- `de90a5c..HEAD`, `55e04e6..17cc346`
#   * an UNBACKTICKED sha inside quoted git output or prose
#     -- "commit 3090b68e990bfb063150cbb46f4c3a93bf574883", "landed in 6197b53"
# The extraction is a single `tr -c` that turns every non-hex byte into a newline, so no
# range/backtick/word-boundary parsing is involved and none of the three shapes can slip
# past: a range's "." separator, a backtick and a space are all equally just separators.
# Consequence worth knowing: a 7+ digit decimal number, or an all-of-a-f English word of
# 7+ letters, would also be treated as a cited sha and reported FAIL. That is the
# intended trade (a false FAIL is loud and fixable by quoting the number differently; a
# missed broken sha is silent), and no such token exists in the three documents today.
cat $DOCS | tr -c '0-9a-f' '\n' | grep -xE '[0-9a-f]{7,40}' | sort -u > "$tmpdir/shas.txt"
n=0
c1=0
while read -r s; do
  [ -n "$s" ] || continue
  n=$((n + 1))
  if ! git cat-file -e "${s}^{commit}" 2>/dev/null; then
    fail "cited sha $s does not resolve (git cat-file -e)"
    c1=1
  fi
done < "$tmpdir/shas.txt"
[ "$c1" -eq 0 ] && pass "check 1: all $n distinct cited shas resolve (git cat-file -e)"

# --- check 2: every path cited in the three documents is tracked ------------------
# The scan is deliberately WIDER than "a backtick span containing no spaces": every
# backtick span is split on whitespace and EVERY resulting token is considered, so a
# path cited inside a multi-token command span is checked exactly like a path cited
# on its own. That is what makes all of these shapes checkable rather than only the
# first:
#   * a path quoted on its own          -- `internal/tmux/geometry.go`
#   * a path inside a quoted command    -- `ci/run.sh go test -count=1 ./internal/tui/`,
#                                          `ci/stability.sh 10`
#   * a markdown link target            -- [..](phase3h-findings.md), [..](reports/phase3i.md)
# (An earlier revision of this script extracted only complete space-free backtick
# spans with `grep -oh '`[^` ]*`'`, so the nine `ci/run.sh` / `ci/stability.sh`
# citations that only ever appear inside multi-token command spans were never checked
# at all: they could have gone stale silently. Hence the split.)
#
# A token is treated as a cited PATH when, after stripping wrapping quotes, trailing
# markdown/prose punctuation and any `:NNN`/`:NNN-NNN` line-number suffix, it either
# contains a `/` or ends in one of this repo's file extensions, or is a
# `phase3i*`/`phase3h*` report-directory shorthand. Everything else in a command span
# (`go`, `test`, `-count=1`, `grep`, `sed`) is not path-shaped and is skipped.
#
# Five documented exclusions, each a token that is path-SHAPED but is not a path in
# this repo:
#   * `tasks.json` and any `/run/ralphd/...` token -- the run's OWN state files, which
#     live outside this repo by design and are quoted from prose describing the run.
#   * `origin/main` -- a git ref (it is quoted inside check 4's own command), not a path.
#   * `./...` -- the Go package pattern, not a directory.
#   * a token holding a shell/placeholder metacharacter (`<>$=&|;(){}!^@#~`) -- command
#     fragments such as `2>&1`, `DECK_GODOG_PATHS=<file>.feature` and `~/.git-credentials`
#     (the last a real path, but in $HOME, not in the repo).
#   * a token made only of digits and `/` -- a ratio such as `10/10`, not a path.
# Ordinary shorthand basenames like `ownership.go` or `tui.go` are NOT excluded — they
# genuinely resolve via the basename fallback below, against their one real tracked file.
# Glob citations (`*.go`, `*.feature`) are not excluded either: git resolves them as
# pathspecs, so an extension that stopped existing in the tree would FAIL loudly.
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

for raw in $(grep -oh '`[^`]*`' $DOCS | tr -d '`' | tr -s ' \t' '\n' \
               | sed "s/^['\"]*//; s/['\",;)]*\$//; s/:[0-9]*-\{0,1\}[0-9]*\$//" | sort -u); do
  case "$raw" in
    tasks.json|origin/main|'./...') continue ;;
    ''|/) continue ;;
    '~'*|'#'*|/run/ralphd/*) continue ;;
    *[\<\>\$\=\&\|\;\(\)\{\}\!\^\@\#\~]*) continue ;;
  esac
  # digits-and-slashes only (a ratio like 10/10) is not a path
  case "$raw" in
    *[!0-9/]*) ;;
    *) continue ;;
  esac
  case "$raw" in
    *.md|*.go|*.feature|*.sh|*.toml|*.log|*.exitstatus|*/) ;;
    phase3i*|phase3h*) ;;
    */*) ;;
    *) continue ;;
  esac
  m=$((m + 1))
  if ! check_one_path "$raw"; then
    fail "cited path $raw is not tracked (git ls-files --error-unmatch)"
    c2=1
  fi
done

for l in $(grep -oh ']([^) ]*)' $DOCS | sed 's/^](//; s/)$//' | sort -u); do
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
