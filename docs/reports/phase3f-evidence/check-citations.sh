#!/bin/sh
# Verify every sha cited in the phase 3f reports resolves (git cat-file -e), every
# relative path they link to exists, and every backticked repo-relative SOURCE path
# (a directory-qualified *.go/*.toml/*.feature/*.sh/*.sql/*.md, with an optional
# ":line" or ":line-line" suffix) names a file that exists -- so a report cannot cite
# a test in the wrong package and still pass. Ten-digit decimals are unix timestamps,
# not shas, and are excluded. Run from the repo root; pass report paths to override
# the default pair.
set -u
[ $# -gt 0 ] && files="$*" || files="docs/reports/phase3f.md docs/reports/phase3f-findings.md"
bad=0
n=0
m=0
k=0
for f in $files; do
  [ -e "$f" ] || { echo "MISSING REPORT: $f"; bad=1; continue; }
  shas=$( { grep -oE '`[0-9a-f]{7,40}`' "$f" | tr -d '`';
            grep -oE '^[0-9a-f]{7,40} [0-9]{2}:[0-9]{2} ' "$f" | cut -d' ' -f1;
          } | grep -vE '^[0-9]{10}$' | sort -u )
  for s in $shas; do
    n=$((n+1))
    git cat-file -e "$s^{commit}" 2>/dev/null || { echo "UNRESOLVED SHA in $f: $s"; bad=1; }
  done
  for p in $(grep -oE '\]\([A-Za-z0-9._/-]+\)' "$f" | sed -E 's/^\]\(//; s/\)$//' | sort -u); do
    m=$((m+1))
    [ -e "docs/reports/$p" ] || { echo "MISSING PATH cited by $f: docs/reports/$p"; bad=1; }
  done
  # Backticked source paths. A path passes if it exists at the repo root, or under
  # docs/reports/ (report-relative evidence links), or existed at some commit in
  # history (a deliberately cited PRE-RENAME name, e.g. the eleven files R67 moved).
  # The history escape is discriminating, not a loophole: it admits
  # internal/config/config_task017_test.go (real until b848d28) while still rejecting
  # internal/tui/golden_frame_test.go, a package that never held that test -- the
  # miscitation this check was added to catch. Not checked, on purpose: tokens
  # carrying a brace/glob/ellipsis or an angle bracket (deliberately abbreviated,
  # e.g. builtin/{cobalt,daylight}.toml) and bare filenames with no directory
  # component (ambiguous by construction) -- the grep below cannot match the former
  # and the grep '/' filter drops the latter.
  srcs=$( grep -oE '`[A-Za-z0-9._/-]+\.(go|toml|feature|sh|sql|md)(:[0-9]+(-[0-9]+)?)?`' "$f" \
          | tr -d '`' | sed -E 's/:[0-9]+(-[0-9]+)?$//' | grep '/' | sort -u )
  for p in $srcs; do
    k=$((k+1))
    [ -e "$p" ] && continue
    [ -e "docs/reports/$p" ] && continue
    [ -n "$(git log --oneline -1 --all -- "$p" 2>/dev/null)" ] && continue
    echo "MISSING SOURCE PATH cited by $f: $p"; bad=1
  done
done
echo "checked $n shas, $m cited paths and $k backticked source paths across: $files"
[ $bad -eq 0 ] && echo "ALL CITED SHAS RESOLVE, ALL CITED PATHS EXIST AND EVERY BACKTICKED SOURCE PATH EXISTS"
exit $bad
