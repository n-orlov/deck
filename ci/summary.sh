#!/bin/sh
# ci/summary.sh — renders a GitHub Actions job summary (pass/fail/skip/flaky
# counts, slowest packages) from one ci/suite.sh output directory.
#
#   ci/summary.sh <ci/suite.sh output dir>  >> "$GITHUB_STEP_SUMMARY"
#
# Reads exactly the files ci/suite.sh already writes:
#   junit-merged/junit-go.xml, junit-merged/junit-features.xml (or, in a
#   results dir written before junit-merged/ existed, the top-level
#   junit-go.xml, junit-features.xml)
#                                       — every <testcase> for the pass/fail/
#                                         skip counts, one test per distinct
#                                         test with its last attempt's
#                                         outcome, exactly as the Allure
#                                         report counts the same tests
#                                         (count_junit below), and
#                                         each <testsuite name= time=> child
#                                         (one Go package, or one Godog
#                                         feature file) for the slowest-N
#                                         table.
#   flaky-go.txt, flaky-features.txt   — one line per test/scenario that
#                                         failed once then passed on retry;
#                                         counted, and listed by name.
#   coverage-summary.txt                — ci/suite.sh's own package-by-package
#                                         table (unit + features/ black-box),
#                                         reprinted verbatim (R146 criterion 3).
# A missing file (e.g. features/ never ran because the unit pass itself
# never produced output) is skipped rather than treated as an error, so a
# partial ci-results directory still renders whatever it has.
#
# An optional second argument is the Allure report link (R146 criterion 3);
# omitted, the summary simply has no "Allure report" line.
set -eu

outdir=${1:?"usage: ci/summary.sh <ci/suite.sh output dir> [<allure report link>]"}
report_link=${2:-}

total_pass=0
total_failures=0
total_skipped=0
flaky_count=0

junit_dir="$outdir/junit-merged"
[ -d "$junit_dir" ] || junit_dir=$outdir

# count_junit prints "<passed> <failed> <skipped>" for one JUnit file,
# counting the way Allure does: one test per (testsuite name, testcase
# classname, testcase name) -- the key ci/junitflaky merges attempts by and
# ci/junit2allure derives a historyId from -- its outcome the LAST attempt's.
# So a test that failed on every attempt is one failure, not one per attempt.
# An attempt's outcome follows ci/junitflaky's outcomeOf: a <failure> or
# <error> child fails it, a <skipped> child skips it, otherwise godog's
# status attribute decides (failed/undefined/pending/ambiguous fail, skipped
# skips), otherwise it passed. <rerunFailure>/<rerunError> children are
# earlier attempts of a test that passed on retry and do not fail it.
count_junit() {
    awk 'BEGIN { RS = "<"; n = 0; key = "" }
    function attr(rec, name,   re) {
        re = "[ \t\n]" name "=\"[^\"]*\""
        if (match(rec, re)) return substr(rec, RSTART + length(name) + 3, RLENGTH - length(name) - 4)
        return ""
    }
    function close_case(   st) {
        if (key == "") return
        if (outcome == "") {
            st = status
            if (st == "failed" || st == "undefined" || st == "pending" || st == "ambiguous") outcome = "fail"
            else if (st == "skipped") outcome = "skip"
            else outcome = "pass"
        }
        if (!(key in last)) n++
        last[key] = outcome
        key = ""
    }
    /^testsuite[ \t\n\/>]/ { suite = attr($0, "name"); next }
    /^testcase[ \t\n\/>]/ {
        close_case()
        key = suite SUBSEP attr($0, "classname") SUBSEP attr($0, "name")
        status = attr($0, "status"); outcome = ""
        if ($0 ~ /\/>/) close_case()
        next
    }
    /^\/testcase>/ { close_case(); next }
    key != "" && /^(failure|error)[ \t\n\/>]/ { outcome = "fail"; next }
    key != "" && /^skipped[ \t\n\/>]/ { if (outcome == "") outcome = "skip"; next }
    END {
        close_case()
        p = 0; f = 0; s = 0
        for (k in last) {
            if (last[k] == "fail") f++
            else if (last[k] == "skip") s++
            else p++
        }
        printf "%d %d %d\n", p, f, s
    }' "$1"
}

for junit in "$junit_dir/junit-go.xml" "$junit_dir/junit-features.xml"; do
    [ -f "$junit" ] || continue
    counts=$(count_junit "$junit")
    set -- $counts
    total_pass=$((total_pass + ${1:-0}))
    total_failures=$((total_failures + ${2:-0}))
    total_skipped=$((total_skipped + ${3:-0}))
done

for flaky in "$outdir/flaky-go.txt" "$outdir/flaky-features.txt"; do
    [ -f "$flaky" ] || continue
    n=$(grep -c . "$flaky" 2>/dev/null || true)
    flaky_count=$((flaky_count + ${n:-0}))
done

echo "### CI suite summary"
echo
if [ -n "$report_link" ]; then
    echo "Allure report: $report_link"
    echo
fi
echo "| metric | count |"
echo "| --- | --- |"
echo "| pass | $total_pass |"
echo "| fail | $total_failures |"
echo "| skip | $total_skipped |"
echo "| flaky | $flaky_count |"
echo
if [ "$flaky_count" -gt 0 ]; then
    echo "#### flaky (failed, then passed on retry)"
    echo
    for flaky in "$outdir/flaky-go.txt" "$outdir/flaky-features.txt"; do
        [ -f "$flaky" ] || continue
        grep . "$flaky" | sed 's/^/- /' || true
    done
    echo
fi
echo "#### slowest packages / feature files"
echo

times=$(mktemp)
trap 'rm -f "$times"' EXIT

for junit in "$junit_dir/junit-go.xml" "$junit_dir/junit-features.xml"; do
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

if [ -f "$outdir/coverage-summary.txt" ]; then
    echo
    echo "#### coverage (unit -coverprofile, features/ black-box GOCOVERDIR)"
    echo
    echo '```'
    cat "$outdir/coverage-summary.txt"
    echo '```'
fi
