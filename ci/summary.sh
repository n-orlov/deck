#!/bin/sh
# ci/summary.sh — renders a GitHub Actions job summary (pass/fail/flaky
# counts, slowest packages) from one ci/suite.sh output directory.
#
#   ci/summary.sh <ci/suite.sh output dir>  >> "$GITHUB_STEP_SUMMARY"
#
# Reads exactly the files ci/suite.sh already writes:
#   junit-go.xml, junit-features.xml   — <testsuites tests= failures= errors=>
#                                         root attributes for the counts, and
#                                         each <testsuite name= time=> child
#                                         (one Go package, or one Godog
#                                         feature file) for the slowest-N
#                                         table.
#   flaky-go.txt, flaky-features.txt   — one line per test/scenario that
#                                         failed once then passed on retry.
# A missing file (e.g. features/ never ran because the unit pass itself
# never produced output) is skipped rather than treated as an error, so a
# partial ci-results directory still renders whatever it has.
set -eu

outdir=${1:?"usage: ci/summary.sh <ci/suite.sh output dir>"}

total_tests=0
total_failures=0
flaky_count=0

for junit in "$outdir/junit-go.xml" "$outdir/junit-features.xml"; do
    [ -f "$junit" ] || continue
    line=$(grep -m1 '<testsuites ' "$junit" || true)
    [ -n "$line" ] || continue
    t=$(printf '%s' "$line" | grep -oE 'tests="[0-9]+"' | head -1 | grep -oE '[0-9]+' || true)
    f=$(printf '%s' "$line" | grep -oE 'failures="[0-9]+"' | head -1 | grep -oE '[0-9]+' || true)
    e=$(printf '%s' "$line" | grep -oE 'errors="[0-9]+"' | head -1 | grep -oE '[0-9]+' || true)
    total_tests=$((total_tests + ${t:-0}))
    total_failures=$((total_failures + ${f:-0} + ${e:-0}))
done

for flaky in "$outdir/flaky-go.txt" "$outdir/flaky-features.txt"; do
    [ -f "$flaky" ] || continue
    n=$(grep -c . "$flaky" 2>/dev/null || true)
    flaky_count=$((flaky_count + ${n:-0}))
done

total_pass=$((total_tests - total_failures))

echo "### CI suite summary"
echo
echo "| metric | count |"
echo "| --- | --- |"
echo "| pass | $total_pass |"
echo "| fail | $total_failures |"
echo "| flaky | $flaky_count |"
echo
echo "#### slowest packages / feature files"
echo

times=$(mktemp)
trap 'rm -f "$times"' EXIT

for junit in "$outdir/junit-go.xml" "$outdir/junit-features.xml"; do
    [ -f "$junit" ] || continue
    grep -oE '<testsuite [^>]*>' "$junit" | while IFS= read -r line; do
        name=$(printf '%s' "$line" | grep -oE 'name="[^"]*"' | head -1 | sed -E 's/^name="//; s/"$//')
        tm=$(printf '%s' "$line" | grep -oE 'time="[0-9.]+"' | head -1 | grep -oE '[0-9.]+')
        [ -n "$name" ] && [ -n "$tm" ] && printf '%s %s\n' "$tm" "$name"
    done
done >> "$times"

if [ -s "$times" ]; then
    echo "| package / feature | time (s) |"
    echo "| --- | --- |"
    sort -rn "$times" | head -10 | while IFS=' ' read -r tm rest; do
        printf '| %s | %s |\n' "$rest" "$tm"
    done
else
    echo "(no timed testsuite entries found)"
fi
