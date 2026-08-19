#!/bin/sh
# Run the full test suite N times, from a clean state each run, and label
# each run PASS/FAIL from the ACTUAL exit status of `go test` itself.
#
#   ci/stability.sh        # 10 runs (default)
#   ci/stability.sh 5      # 5 runs
#
# Gotcha this script exists to avoid: piping `go test` into `tee` and then
# checking `$?` observes tee's exit status, not go test's, so a real test
# failure can be silently mislabelled PASS (see docs/reports/
# phase1-findings.md, "ten-run-loop mislabelled-PASS defect"). This script
# NEVER pipes the command whose exit status matters; it redirects to a file
# and reads `$?` immediately after the command line that produced it, then
# only afterwards echoes the captured file for display/logging.
#
# Each run is "clean state": -count=1 disables Go's test result cache, each
# run gets its own fresh log file (no previous run's output is reused or
# appended into the same buffer before the pass/fail decision), and
# ci/run.sh's sibling container is --rm per invocation so any tmux sockets
# or other run-scoped state inside it die with the container at the end of
# that one run and cannot leak into the next. -p=1 serializes Go packages:
# the black-box package deliberately asserts sub-second tmux/SQLite deadlines,
# so loading it beside the separate full Godog package would turn a stability
# run into a host-scheduler benchmark rather than ten repetitions of the same
# uncontended product contract. Tests and scenarios within each package remain
# unchanged and fully executed.
#
# Self-test / demonstration hook: set DECK_STABILITY_SELFTEST_FAIL=1 to make
# every run a deliberate injected failure instead of invoking the real
# suite, to prove the FAIL path (and non-zero script exit) actually fires.
set -u

runs=${1:-10}
case "$runs" in
    ''|*[!0-9]*)
        echo "usage: ci/stability.sh [N]  (N must be a positive integer, got '$runs')" >&2
        exit 2
        ;;
esac
if [ "$runs" -lt 1 ]; then
    echo "usage: ci/stability.sh [N]  (N must be >= 1, got '$runs')" >&2
    exit 2
fi

repo_root=$(cd "$(dirname "$0")/.." && pwd)
outdir=$(mktemp -d "${TMPDIR:-/tmp}/deck-stability.XXXXXX")
summary_log="$outdir/summary.log"
: > "$summary_log"

pass=0
fail=0

i=1
while [ "$i" -le "$runs" ]; do
    run_log="$outdir/run-$i.log"

    echo "=== RUN $i ===" | tee -a "$summary_log"

    if [ "${DECK_STABILITY_SELFTEST_FAIL:-0}" = "1" ]; then
        # Deliberate injected failure path: proves the FAIL label and
        # non-zero script exit actually fire, without needing a real bug.
        echo "DECK_STABILITY_SELFTEST_FAIL=1: injected failure (self-test only, real suite not run)" > "$run_log"
        status=1
    else
        # NOT piped: this is the real exit status of go test, captured
        # immediately, before any tee/cat touches the output.
        (cd "$repo_root" && "$repo_root/ci/run.sh" go test -p=1 -count=1 ./...) > "$run_log" 2>&1
        status=$?
    fi

    cat "$run_log" | tee -a "$summary_log" >/dev/null

    if [ "$status" -eq 0 ]; then
        echo "=== RUN $i: PASS (exit $status) ===" | tee -a "$summary_log"
        pass=$((pass + 1))
    else
        echo "=== RUN $i: FAIL (exit $status) ===" | tee -a "$summary_log"
        fail=$((fail + 1))
    fi

    i=$((i + 1))
done

echo "full per-run logs and combined summary log kept in: $outdir" | tee -a "$summary_log"
echo "$pass/$runs passed" | tee -a "$summary_log"

if [ "$fail" -gt 0 ]; then
    exit 1
fi
exit 0
