#!/bin/sh
# Task 204 evidence harness: 20 consecutive targeted runs of
# filter.feature's @requirement-33-dd-reaches-and-tombstones-an-archived-row
# scenario, executed while a SECOND suite invocation runs in parallel, so the
# scheduling pressure of stability run 7 is reproduced.
#
# Every claim this produces is checkable from the logs alone:
#   - the parallel suite writes its own log (parallel-suite.log) with its
#     start/end wall-clock timestamps, its sibling container id and its exit code;
#   - every targeted run records its own start/end timestamps AND a `docker ps`
#     snapshot of this run's sibling containers taken while it is executing, in
#     which the parallel suite's container id must appear (OVERLAP=yes) or not
#     (OVERLAP=no). No narrative is needed to establish overlap.
#
# Usage (from the repository root, inside the ralphd job container):
#   sh docs/reports/phase3g-204-async-db-assert-sync/parallel-pressure-runs.sh
set -u

dir=docs/reports/phase3g-204-async-db-assert-sync
runs_log=$dir/20-consecutive-runs.log
suite_log=$dir/parallel-suite.log
targeted='ci/run.sh env DECK_GODOG_PATHS=filter.feature go test ./features/ -run TestFeatures -count=1'
sibling_filters="--filter label=ralphd.run=${RALPHD_RUN_ID:-none} --filter label=ralphd.role=sibling"

ts() { date -u +%H:%M:%S.%3N; }
siblings() { docker ps $sibling_filters --format '{{.ID}} started={{.RunningFor}} cmd={{.Command}}'; }

: > "$runs_log"
{
    echo "harness: $dir/parallel-pressure-runs.sh"
    echo "host uname: $(uname -sr)  nproc: $(nproc)"
    echo "git HEAD: $(git rev-parse HEAD)  dirty: $(git status --porcelain | wc -l) file(s)"
    echo "targeted command (x20): $targeted"
    echo "parallel second suite invocation: ci/run.sh go test ./features/ -run TestFeatures -count=1 (whole godog suite, all feature files)"
    echo "all timestamps UTC HH:MM:SS.mmm"
} >> "$runs_log"

# --- start the second (parallel) suite invocation -------------------------
{
    echo "=== parallel suite invocation start $(ts) ==="
    echo "command: ci/run.sh go test ./features/ -run TestFeatures -count=1"
} > "$suite_log"
ci/run.sh go test ./features/ -run TestFeatures -count=1 >> "$suite_log" 2>&1 &
suite_pid=$!
echo "parallel suite: shell pid=$suite_pid started $(ts)" >> "$runs_log"

# Resolve the parallel suite's own sibling container id (it is the only sibling
# running at this point; the targeted runs each start their own afterwards).
suite_cid=""
i=0
while [ $i -lt 60 ]; do
    suite_cid=$(docker ps $sibling_filters --format '{{.ID}}' | head -1)
    [ -n "$suite_cid" ] && break
    sleep 1
    i=$((i + 1))
done
{
    echo "parallel suite: sibling container id=$suite_cid"
    echo "parallel suite: docker inspect StartedAt=$(docker inspect -f '{{.State.StartedAt}}' "$suite_cid" 2>/dev/null) Cmd=$(docker inspect -f '{{.Config.Cmd}}' "$suite_cid" 2>/dev/null)"
    echo
} >> "$runs_log"

# --- 20 consecutive targeted runs ----------------------------------------
green=0
overlapped=0
n=1
while [ $n -le 20 ]; do
    start=$(ts)
    out=$(eval "$targeted" 2>&1)
    code=$?
    # Snapshot taken immediately after the run's own sibling exits; the
    # parallel suite's container is still listed iff it is still executing.
    snap=$(siblings)
    still=$(docker ps $sibling_filters --format '{{.ID}}' | grep -c "^$suite_cid")
    if [ "$still" -ge 1 ]; then overlap=yes; overlapped=$((overlapped + 1)); else overlap=no; fi
    [ "$code" -eq 0 ] && green=$((green + 1))
    {
        echo "=== run $n start=$start end=$(ts) exit=$code OVERLAP=$overlap ==="
        echo "$out" | grep -E '^(ok|FAIL|---|\s*step error)' || echo "$out" | tail -3
        echo "-- docker ps (this run's siblings) during/at end of run $n:"
        echo "$snap" | sed 's/^/   /'
        echo "-- parallel suite pid $suite_pid alive: $(ps -p "$suite_pid" >/dev/null 2>&1 && echo yes || echo no)"
    } >> "$runs_log"
    n=$((n + 1))
done

# --- close out the parallel suite ----------------------------------------
{
    echo
    echo "loop finished $(ts): green=$green/20 overlapped_with_parallel_suite=$overlapped/20"
    echo "waiting for the parallel suite invocation to finish..."
} >> "$runs_log"
wait "$suite_pid"
suite_code=$?
{
    echo "=== parallel suite invocation end $(ts) exit=$suite_code ==="
} >> "$suite_log"
{
    echo "parallel suite exit=$suite_code (full output: parallel-suite.log)"
    echo "SUMMARY green=$green/20 overlapped=$overlapped/20 parallel_suite_exit=$suite_code"
} >> "$runs_log"
