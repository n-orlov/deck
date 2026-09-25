#!/bin/sh
# ci/suite.sh — the CI suite runner (R145): drives the whole test matrix
# (`go test -p=1 -count=1 ./...`, in effect) through gotestsum, fetched at a
# pinned version rather than vendored or added to go.mod (`go run
# gotest.tools/gotestsum@$GOTESTSUM_VERSION`), so this file is the only
# place the version is bumped and ci/Dockerfile never needs to know
# gotestsum exists. It writes standard JUnit for the ordinary Go package
# matrix and Godog's own JUnit for features/, and applies a
# fail-once-retry-once policy throughout: a test (or scenario) that fails
# once and then passes is recorded to a flaky list and the run exits 0; one
# that fails twice makes this script exit non-zero.
#
#   ci/run.sh ci/suite.sh                  # from a sandbox without local Go
#   ci/suite.sh                            # on a runner with Go on PATH already
#   DECK_CI_OUT=/path/to/artifacts ci/suite.sh   # keep the JUnit/flaky files
#
# Why features/ gets a second, different retry path
# ---------------------------------------------------
# The ordinary Go package matrix is driven with gotestsum's own
# `--rerun-fails=1`: on a failure it reruns exactly the failed test
# (`go test -run '^Name$' <pkg>`), which is precise because in an ordinary Go
# package one test function is one unit of work.
#
# features/ is different: the entire suite is ONE Go test function
# (TestFeatures) that fans out into 300+ Godog scenarios via
# godog.TestSuite. Handing that package to gotestsum's rerun-fails would
# rerun `-run '^TestFeatures$'` on a failure, which re-executes every
# scenario in the package again — exactly the "whole package" rerun task
# 015's criterion (3) forbids. So features/ is excluded from the gotestsum
# pass and run once directly; on failure this script parses the pretty
# formatter's own "--- Failed steps:" summary (always emitted, in both the
# terminal and the JUnit-carrying "pretty,junit:<path>" format — see
# godogFormat() in features/godog_test.go, task 014) for each failed
# scenario's own "<file>:<line>" location, and reruns ONLY that scenario via
# DECK_GODOG_PATHS=<file>:<line> (features/godog_test.go's existing
# godogPaths() selector, task 014) — never the package's default `.` path.
set -eu

GOTESTSUM_VERSION=${GOTESTSUM_VERSION:-v1.13.0}
gotestsum_pkg="gotest.tools/gotestsum@${GOTESTSUM_VERSION}"

repo_root=$(cd "$(dirname "$0")/.." && pwd)
cd "$repo_root"

outdir=${DECK_CI_OUT:-}
if [ -z "$outdir" ]; then
    outdir=$(mktemp -d "${TMPDIR:-/tmp}/deck-ci-suite.XXXXXX")
fi
mkdir -p "$outdir"
echo "ci/suite.sh: writing JUnit/flaky/log files to $outdir"

overall_status=0

# --- 1. ordinary Go packages (everything except features/) via gotestsum ---
# DECK_CI_GO_PACKAGES lets a caller narrow the package list (used by this
# task's own local demonstration, so a scratch worktree's throwaway package
# can be exercised without also re-running the whole real suite); unset,
# the default is every package except features/, i.e. the ordinary case.
pkgs=${DECK_CI_GO_PACKAGES:-$(go list ./... | grep -vE '/features$' || true)}

go_junit="$outdir/junit-go.xml"
go_rerun_report="$outdir/rerun-report-go.txt"
go_flaky="$outdir/flaky-go.txt"
: > "$go_flaky"

if [ -n "$pkgs" ]; then
    go_status=0
    go run "$gotestsum_pkg" \
        --format testname \
        --junitfile "$go_junit" \
        --rerun-fails=1 \
        --rerun-fails-report "$go_rerun_report" \
        --packages "$pkgs" \
        -- -p=1 -count=1 \
        || go_status=$?

    # gotestsum's own rerun report lists every test it reran, whether or not
    # it eventually passed ("<test>: N runs, M failures"). Criterion (2)
    # wants a *flaky* list restricted to tests that failed once and then
    # passed on the rerun (failures < runs); a test where failures == runs
    # never passed at all and is reflected in go_status instead.
    if [ -f "$go_rerun_report" ]; then
        while IFS= read -r line; do
            runs=$(printf '%s\n' "$line" | grep -oE '[0-9]+ runs' | grep -oE '[0-9]+' || true)
            fails=$(printf '%s\n' "$line" | grep -oE '[0-9]+ failures' | grep -oE '[0-9]+' || true)
            if [ -n "$runs" ] && [ -n "$fails" ] && [ "$fails" -lt "$runs" ]; then
                printf '%s\n' "$line" >> "$go_flaky"
            fi
        done < "$go_rerun_report"
    fi

    if [ "$go_status" -ne 0 ]; then
        echo "ci/suite.sh: Go package matrix failed after retry (exit $go_status)" >&2
        overall_status=1
    fi
else
    echo "ci/suite.sh: no non-features packages found (go list ./... returned only features/)" >&2
fi

# --- 2. features/: one run, then a per-scenario rerun of anything failed ---
features_junit="$outdir/junit-features.xml"
features_log="$outdir/features-run1.log"
features_flaky="$outdir/flaky-features.txt"
: > "$features_flaky"

features_status=0
if ! DECK_GODOG_JUNIT="$features_junit" go test -p=1 -count=1 ./features/ -run '^TestFeatures$' > "$features_log" 2>&1; then
    features_status=1
    clean_log="$outdir/features-run1.clean.log"
    sed -E 's/\x1b\[[0-9;]*m//g' "$features_log" > "$clean_log"

    locations=$(sed -n '/^--- Failed steps:/,$p' "$clean_log" \
        | grep -E '^[[:space:]]*Scenario' \
        | grep -oE '# [^[:space:]]+\.feature:[0-9]+' \
        | sed 's/^# //' \
        | sort -u)

    if [ -z "$locations" ]; then
        echo "ci/suite.sh: features/ failed but no scenario location could be parsed from $features_log" >&2
    else
        all_recovered=1
        i=0
        for loc in $locations; do
            i=$((i + 1))
            rerun_log="$outdir/features-rerun-$i.log"
            echo "ci/suite.sh: features/ rerunning failed scenario alone: DECK_GODOG_PATHS=$loc" >&2
            if DECK_GODOG_PATHS="$loc" go test -p=1 -count=1 ./features/ -run '^TestFeatures$' > "$rerun_log" 2>&1; then
                echo "$loc" >> "$features_flaky"
            else
                echo "ci/suite.sh: features/ scenario $loc failed again on its solo rerun" >&2
                all_recovered=0
            fi
        done
        if [ "$all_recovered" -eq 1 ]; then
            features_status=0
        fi
    fi
fi

if [ "$features_status" -ne 0 ]; then
    echo "ci/suite.sh: features/ failed after per-scenario retry" >&2
    overall_status=1
fi

echo "ci/suite.sh: done (go_flaky=$go_flaky, features_flaky=$features_flaky, outdir=$outdir)"
exit "$overall_status"
