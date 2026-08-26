#!/bin/sh
# Verify every sha cited in docs/reports/phase3f.md resolves (git cat-file -e)
# and every relative path it links to exists. Ten-digit decimals are unix
# timestamps, not shas, and are excluded.
set -u
f=docs/reports/phase3f.md
bad=0
shas=$( { grep -oE '`[0-9a-f]{7,40}`' "$f" | tr -d '`';
          grep -oE '^[0-9a-f]{7,40} [0-9]{2}:[0-9]{2} ' "$f" | cut -d' ' -f1;
        } | grep -vE '^[0-9]{10}$' | sort -u )
n=0
for s in $shas; do
  n=$((n+1))
  git cat-file -e "$s^{commit}" 2>/dev/null || { echo "UNRESOLVED SHA: $s"; bad=1; }
done
m=0
for p in $(grep -oE '\]\([A-Za-z0-9._/-]+\)' "$f" | sed -E 's/^\]\(//; s/\)$//' | sort -u); do
  m=$((m+1))
  [ -e "docs/reports/$p" ] || { echo "MISSING PATH: docs/reports/$p"; bad=1; }
done
echo "checked $n shas and $m cited paths"
[ $bad -eq 0 ] && echo "ALL CITED SHAS RESOLVE AND ALL CITED PATHS EXIST"
exit $bad
