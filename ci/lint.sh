#!/bin/sh
# ci/lint.sh — fail-fast Go lint gate (R145): `gofmt -l` over every tracked
# Go file, then `go vet ./...`, then a `go mod tidy` drift check on
# go.mod/go.sum, in that order, stopping at the first failure so CI reports
# the earliest, cheapest signal rather than piling up unrelated ones.
#
# Assumes `go` (and its bundled `gofmt`) are already on PATH -- true both in
# a GitHub Actions runner (task C.5, .github/workflows/ci.yml) and inside the
# ci/run.sh sibling container used from this sandbox. Invoke it:
#
#   ci/lint.sh              # on a host/runner with Go installed directly
#   ci/run.sh ci/lint.sh     # from a sandbox without a local Go toolchain
set -eu

repo_root=$(cd "$(dirname "$0")/.." && pwd)
cd "$repo_root"

echo "ci/lint.sh: gofmt -l"
fmt_files=$(git ls-files '*.go')
fmt_out=""
if [ -n "$fmt_files" ]; then
    fmt_out=$(gofmt -l $fmt_files)
fi
if [ -n "$fmt_out" ]; then
    echo "ci/lint.sh: gofmt drift in:" >&2
    echo "$fmt_out" >&2
    exit 1
fi

echo "ci/lint.sh: go vet ./..."
go vet ./...

echo "ci/lint.sh: go mod tidy drift check"
tmp_mod=$(mktemp)
tmp_sum=$(mktemp)
cp go.mod "$tmp_mod"
cp go.sum "$tmp_sum"

drift=0
go mod tidy
if ! diff -u "$tmp_mod" go.mod || ! diff -u "$tmp_sum" go.sum; then
    drift=1
fi

# Restore the working tree exactly as it stood before `go mod tidy` ran,
# whether or not drift was found, so this script never leaves a modified
# go.mod/go.sum behind for the caller to notice or commit by accident.
cp "$tmp_mod" go.mod
cp "$tmp_sum" go.sum
rm -f "$tmp_mod" "$tmp_sum"

if [ "$drift" -ne 0 ]; then
    echo "ci/lint.sh: go mod tidy would change go.mod/go.sum -- run it locally and commit the diff" >&2
    exit 1
fi

echo "ci/lint.sh: clean (gofmt, go vet, go mod tidy all pass)"
