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
#   ci/review.sh                  # the authorized no-argument invocation:
#                                 # clone + identity assertion, exit 0, no tests
#   ci/review.sh --smoke          # same as no argument
#   ci/review.sh <go test args...>
#
#   ci/review.sh ./internal/agent/                        # narrow smoke
#   ci/review.sh -p=1 -count=1 -timeout=40m ./...          # full-gate target
#
# With no argument (or the single argument --smoke) the script stops after the
# identity assertion and exits 0; it never defaults to running a test target.
# Any other arguments are passed verbatim to `go test` inside the clone after the
# assertion passed; this script does not choose or narrow that target itself --
# see docs/reports/phase4-review-protocol.md for the invocations a reviewer uses.
#
# It works from any checkout, including a fresh `git clone --no-hardlinks` of this
# repository: the sibling mounts the directory the clone lives in (translated to
# its host path when this runs in a ralphd job container), never a stale
# $RALPHD_HOST_WORKSPACE that names the original workspace.
set -eu

smoke=0
if [ "$#" -eq 0 ] || { [ "$#" -eq 1 ] && [ "$1" = "--smoke" ]; }; then
    smoke=1
fi

repo_root=$(cd "$(dirname "$0")/.." && pwd -P)

# --- choose the directory the sibling mounts at /w (its host path) ---
# ci/run.sh mounts $RALPHD_HOST_WORKSPACE (else $PWD) at /w. Inside a ralphd job
# that variable names the ORIGINAL workspace, so running from another checkout
# would silently measure the original; pin it to the checkout this script is in.
mount_dir=$repo_root
if [ -n "${RALPHD_HOST_WORKSPACE:-}" ]; then
    job_ws=$(cd "${RALPHD_WORKSPACE_DIR:-/workspace}" && pwd -P)
    case "$repo_root" in
        "$job_ws") host_mount=$RALPHD_HOST_WORKSPACE ;;
        "$job_ws"/*) host_mount=$RALPHD_HOST_WORKSPACE${repo_root#"$job_ws"} ;;
        *)
            # Not reachable by the host daemon: stage the clone under the job
            # workspace (its .review-clone is git-ignored) instead.
            mount_dir=$job_ws
            host_mount=$RALPHD_HOST_WORKSPACE
            ;;
    esac
    export RALPHD_HOST_WORKSPACE=$host_mount
fi
clone_dir="$mount_dir/.review-clone"

rm -rf "$clone_dir"
git -C "$repo_root" clone --quiet --local --no-hardlinks "$repo_root" "$clone_dir"

# A nonce only this clone's .git carries: the sibling asserts the package
# directories Go resolved sit in the tree that holds it.
nonce="review-$$-$(date +%s)"
printf '%s\n' "$nonce" > "$clone_dir/.git/review-identity"

echo "review.sh: cloned $repo_root -> $clone_dir"

# --- identity assertion: Go analogue of the reviewer's Python import-location check ---
"$repo_root/ci/run.sh" sh -c '
set -eu
nonce=$1
clone=/w/.review-clone
cd "$clone"
fail() { echo "review.sh: $*" >&2; exit 1; }

[ "$(cat "$clone/.git/review-identity" 2>/dev/null)" = "$nonce" ] ||
    fail "the clone at $clone does not carry this run identity"

mod=$(go list -m)
[ "$mod" = "github.com/n-orlov/deck" ] ||
    fail "module path is [$mod], want github.com/n-orlov/deck"

gomod=$(go env GOMOD)
[ "$gomod" = "$clone/go.mod" ] || fail "GOMOD is [$gomod], want $clone/go.mod"
[ -z "$(go env GOWORK)" ] || fail "a go.work [$(go env GOWORK)] could redirect packages out of the clone"
case "$(go env GOFLAGS)" in
    *-modfile*|*-overlay*) fail "GOFLAGS [$(go env GOFLAGS)] redirects module files" ;;
esac

# Every package of the module (and every module-local dependency of it, tests
# included) must resolve inside the clone: not the original workspace at /w
# itself, and not an installed copy in the module cache.
pkgs=$(go list -test -deps -f "{{if and .Module (eq .Module.Path \"github.com/n-orlov/deck\")}}{{.Dir}}{{end}}" ./... | sort -u)
[ -n "$pkgs" ] || fail "no package of github.com/n-orlov/deck resolved"
for dir in $pkgs; do
    case "$dir" in
        "$clone"/*) : ;;
        *) fail "package dir [$dir] is outside the clone ($clone/...)" ;;
    esac
done
agent=$(go list -f "{{.Dir}}" ./internal/agent)
case "$agent" in
    "$clone"/internal/agent) : ;;
    *) fail "./internal/agent resolved to [$agent], want $clone/internal/agent" ;;
esac
[ "$agent" != "/w/internal/agent" ] ||
    fail "./internal/agent resolved to the original checkout, not the clone"
echo "review.sh: identity ok -- module=$mod dir=$agent packages=$(echo "$pkgs" | wc -l)"
' sh "$nonce"

if [ "$smoke" -eq 1 ]; then
    echo "review.sh: smoke ok (identity only; no test target given)"
    exit 0
fi

# --- caller-supplied go test target, executed inside the same clone ---
echo "review.sh: running go test $* inside the clone"
"$repo_root/ci/run.sh" sh -c 'cd .review-clone && exec go test "$@"' sh "$@"
