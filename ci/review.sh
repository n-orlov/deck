#!/bin/sh
# ci/review.sh — disposable-clone measurement protocol (answers review finding B0).
#
# Review's own template checks a Python package by installing it into a fresh,
# disposable clone/venv and asserting the imported package's __file__ resolves
# under that clone, not under the original checkout — proof the thing under
# test really is the clone's own copy, not something picked up from a stale
# install or cache elsewhere. This repository is a Go module, not a Python
# package (see docs/reports/phase4-review-protocol.md), so there is no
# __file__ to import. This script is the Go analogue of the same check:
#
#   1. Make a disposable, git-ignored clone of this repository
#      ($review's own repo root/.review-clone, removed and recreated each run).
#   2. Inside that clone, through the existing ci/run.sh sibling, assert:
#        - `go list -m` resolves the module path to github.com/n-orlov/deck
#        - `go list -f '{{.Dir}}' ./internal/agent` resolves to a directory
#          under the clone's own /w/.review-clone tree, and NOT to the
#          original checkout's /w/internal/agent — i.e. the package Go is
#          about to build really is the clone's copy, not the original tree.
#   3. Only once that identity assertion passes, run a caller-supplied
#      `go test` target inside the SAME clone, through the same sibling.
#
# Usage:
#   ci/review.sh <go test args...>
#
#   ci/review.sh ./internal/agent/                        # narrow smoke
#   ci/review.sh -p=1 -count=1 -timeout=40m ./...          # full-gate target
#
# Everything after the script name is passed verbatim to `go test` inside the
# clone; this script does not choose or narrow that target itself — see
# docs/reports/phase4-review-protocol.md for the specific invocations a
# reviewer would use.
set -eu

if [ "$#" -lt 1 ]; then
    echo "usage: ci/review.sh <go test args...>" >&2
    exit 2
fi

repo_root=$(cd "$(dirname "$0")/.." && pwd)
clone_dir="$repo_root/.review-clone"

rm -rf "$clone_dir"
git -C "$repo_root" clone --quiet --local --no-hardlinks "$repo_root" "$clone_dir"

echo "review.sh: cloned $repo_root -> $clone_dir"

# --- identity assertion: Go analogue of the reviewer's Python import-location check ---
"$repo_root/ci/run.sh" sh -c '
set -eu
cd .review-clone
mod=$(go list -m)
if [ "$mod" != "github.com/n-orlov/deck" ]; then
    echo "review.sh: module path is [$mod], want github.com/n-orlov/deck" >&2
    exit 1
fi
dir=$(go list -f "{{.Dir}}" ./internal/agent)
case "$dir" in
    /w/.review-clone/*) : ;;
    *)
        echo "review.sh: ./internal/agent resolved to [$dir], want a path under the clone (/w/.review-clone/...)" >&2
        exit 1
        ;;
esac
if [ "$dir" = "/w/internal/agent" ]; then
    echo "review.sh: ./internal/agent resolved to the original checkout, not the clone" >&2
    exit 1
fi
echo "review.sh: identity ok -- module=$mod dir=$dir"
'

# --- caller-supplied go test target, executed inside the same clone ---
echo "review.sh: running go test $* inside the clone"
"$repo_root/ci/run.sh" sh -c 'cd .review-clone && exec go test "$@"' sh "$@"
