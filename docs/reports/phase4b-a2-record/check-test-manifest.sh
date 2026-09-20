#!/bin/sh
# Resolves every line of test-manifest.txt (committed beside this script)
# against the tree at the pinned code revision (task 005's REV, restated
# below and cross-checked against the four tree-object hashes that pin it).
# No suite is run: each line is resolved by inspecting the tree with git and
# grep only. Exit 0 iff every manifest line resolves.
#
# Usage: sh docs/reports/phase4b-a2-record/check-test-manifest.sh
set -eu

REV="70c7430df3b23a46fb735e8573e26ec55908adeb"
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
MANIFEST="$SCRIPT_DIR/test-manifest.txt"

cd "$(git rev-parse --show-toplevel)"

# Cross-check the pin itself before resolving anything against it.
for pair in "internal:15506734d4989e111e871a419ebf46c94a3b59a3" \
            "cmd:27ff2eba72ef6a63a6cf49285c4ddc6660b7fb0d" \
            "features:b5dbe2f565eb96b2654f8d1d64fcc51eb711f60d" \
            "ci:0a183631a2beea070ab0f7d8fa027aecf423e7b0"; do
  path=${pair%%:*}
  want=${pair#*:}
  got=$(git rev-parse "${REV}:${path}")
  if [ "$got" != "$want" ]; then
    echo "TREE HASH MISMATCH at $path: got $got, want $want" >&2
    exit 1
  fi
done
echo "tree-object hashes at $REV: all four equal the pin"

fail=0
total=0
while IFS= read -r line || [ -n "$line" ]; do
  [ -z "$line" ] && continue
  total=$((total + 1))
  case "$line" in
    features/*.feature\ *)
      file=${line%% *}
      title=${line#* }
      content=$(git show "${REV}:${file}" 2>/dev/null) || {
        echo "MISSING FEATURE FILE: $file (line: $line)" >&2
        fail=1
        continue
      }
      if printf '%s\n' "$content" | grep -qF "Scenario: ${title}"; then
        echo "OK   $line"
      else
        echo "MISSING SCENARIO: $line" >&2
        fail=1
      fi
      ;;
    *)
      pkg=${line%% *}
      name=${line#* }
      found=0
      for f in $(git ls-tree -r "$REV" --name-only -- "$pkg" | grep '_test\.go$'); do
        d=$(dirname "$f")
        [ "$d" = "$pkg" ] || continue
        if git show "${REV}:${f}" 2>/dev/null | grep -q "^func ${name}("; then
          found=1
          echo "OK   $line ($f)"
          break
        fi
      done
      if [ "$found" != "1" ]; then
        echo "MISSING FUNC: $line" >&2
        fail=1
      fi
      ;;
  esac
done < "$MANIFEST"

echo "checked $total manifest lines"
if [ "$fail" != "0" ]; then
  echo "FAIL: one or more manifest lines did not resolve" >&2
  exit 1
fi
echo "PASS: every manifest line resolves at $REV"
exit 0
