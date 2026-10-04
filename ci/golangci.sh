#!/bin/sh
# ci/golangci.sh -- the ONE golangci-lint invocation (R188): the whole tree,
# tests included, under the checked-in .golangci.yml, which golangci-lint
# finds from the repository root. Both gate entry points run exactly this:
# ci/lint.sh (the `lint` job, before the suite) and ci/quality.sh (through
# ci/qualitycheck's golangci gate). It takes no arguments and has no
# threshold: any finding is a failure, and so is a run that lints nothing.
set -eu

repo_root=$(cd "$(dirname "$0")/.." && pwd)
cd "$repo_root"

# The deck-ci image sets GOTOOLCHAIN=local; keep that true outside it too so
# the linter never downloads a different toolchain than the one that built
# the tree.
GOTOOLCHAIN=${GOTOOLCHAIN:-local}
export GOTOOLCHAIN

exec golangci-lint run ./...
