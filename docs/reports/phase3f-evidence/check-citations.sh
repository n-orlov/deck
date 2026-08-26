#!/bin/sh
# Verify every sha cited in the phase 3f reports resolves (git cat-file -e) and
# every relative path they link to exists. Ten-digit decimals are unix
# timestamps, not shas, and are excluded. Run from the repo root; pass report
# paths to override the default pair.
set -u
[ $# -gt 0 ] && files="$*" || files="docs/reports/phase3f.md docs/reports/phase3f-findings.md"
bad=0
n=0
m=0
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
done
echo "checked $n shas and $m cited paths across: $files"
[ $bad -eq 0 ] && echo "ALL CITED SHAS RESOLVE AND ALL CITED PATHS EXIST"
exit $bad
