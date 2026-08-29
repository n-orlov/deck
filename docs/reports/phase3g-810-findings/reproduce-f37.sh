#!/bin/sh
# Standalone reproduction of finding F37 (docs/reports/phase3g-findings.md):
# features/sort_order.feature's attention scenario was red at the pre-fix commit
# bedb65a because the SPEC 7 live-pane repair promoted the raw `error` row to
# `running` before the order table was read.
#
# Run it from the repository root with no arguments and no manual editing:
#
#   sh docs/reports/phase3g-810-findings/reproduce-f37.sh
#
# It exits 1 (the reproduced red) and prints the failing step's own text.
# Everything it needs is tracked in the repository: the pre-fix commit, the
# forcing patch next to this script, and ci/run.sh's toolchain sibling.
#
# What it does:
#   1. checks out the pre-fix commit bedb65a into a throwaway git worktree
#      inside the workspace (ci/run.sh only mounts the workspace, so the
#      worktree cannot live in /tmp);
#   2. applies f37-force-repair-race.patch, which inserts ONE already-existing
#      step -- `after one configured reconcile interval deck client "A" screen
#      still contains "waiting"` -- ahead of the order table, so the assertion
#      can no longer outrun the reconcile tick that fires the repair. The patch
#      changes no assertion, no table row and no other scenario; forcing the
#      race is what makes the latent defect observable at all;
#   3. runs the one feature file in a throwaway CI sibling;
#   4. removes the worktree and exits with the test's own status.
#
# The workspace tree is never mutated: the worktree is a separate directory and
# is removed on every exit path, including failure.
set -eu

root=$(git rev-parse --show-toplevel)
cd "$root"

base=${F37_BASE_COMMIT:-bedb65a}
wt=.scratch-810-f37-repro
patch=$root/docs/reports/phase3g-810-findings/f37-force-repair-race.patch

cleanup() {
    git worktree remove --force "$wt" >/dev/null 2>&1 || true
    git worktree prune >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

cleanup
git worktree add --detach "$wt" "$base" >&2
git -C "$wt" apply "$patch"

set +e
ci/run.sh sh -c "cd $wt && env DECK_GODOG_PATHS=sort_order.feature go test ./features/ -run TestFeatures -count=1"
status=$?
set -e

echo "reproduce-f37.sh: go test exit status = $status (expected 1 = red reproduced)" >&2
exit "$status"
