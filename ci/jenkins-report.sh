#!/bin/sh
# ci/jenkins-report.sh — the Jenkins build's reports, built after every
# build (the Jenkinsfile's `post { always }`) from what ci/suite.sh,
# ci/quality.sh and ci/stability.sh left behind:
#
#   allure-report/       the Allure report (ci/allure-report.sh), published
#                        as the build's "Allure" HTML report
#   allure-history/      that report's history/, archived so the next build
#                        of the same branch carries the trend on
#   ci-summary/index.html  ci/summary.sh's counts and slowest packages, the
#                        nightly stability verdicts and the quality gate
#                        report, published as the build's "CI summary"
#   ci-summary/counts.txt  pass=, fail=, skip=, flaky=, coverage= lines the
#                        Jenkinsfile reads for the build page's summary line
#
#   ci/jenkins-report.sh <ci/suite.sh output dir> [<previous allure-history dir>]
#
# Runs on the agent host, not through ci/run.sh: Allure needs a JRE, which
# the Jenkins agent already has and ci/Dockerfile does not carry. Never
# fails because a results file is missing (lint failed, the suite never
# ran): whatever exists is reported.
set -eu

outdir=${1:?"usage: ci/jenkins-report.sh <ci/suite.sh output dir> [<previous allure-history dir>]"}
history_dir=${2:-}
here=$(cd "$(dirname "$0")" && pwd)

rm -rf allure-report allure-history ci-summary
mkdir -p ci-summary

have_results=0
if [ -d "$outdir/allure-results" ] || [ -d "$outdir/junit-merged" ]; then
    have_results=1
fi

if [ "$have_results" -eq 1 ]; then
    sh "$here/allure-report.sh" "$outdir" allure-report "$history_dir"
    if [ -d allure-report/history ]; then
        cp -r allure-report/history allure-history
    fi
fi

html_escape() {
    sed -e 's/&/\&amp;/g' -e 's/</\&lt;/g' -e 's/>/\&gt;/g'
}

summary="ci-summary/summary.md"
if [ "$have_results" -eq 1 ]; then
    report_link=""
    [ -n "${BUILD_URL:-}" ] && report_link="${BUILD_URL}Allure/"
    sh "$here/summary.sh" "$outdir" "$report_link" > "$summary"
else
    echo "(no suite results: the build stopped before ci/suite.sh wrote any)" > "$summary"
fi

{
    echo '<!doctype html><html><head><meta charset="utf-8"><title>deck CI summary</title>'
    echo '<style>body{font-family:system-ui,sans-serif;margin:1.5em}pre{background:#f6f8fa;padding:1em;overflow-x:auto}</style></head><body>'
    echo '<h2>Suite</h2><pre>'
    html_escape < "$summary"
    echo '</pre>'
    for s in "$outdir"/stability/*/summary.log; do
        [ -f "$s" ] || continue
        echo '<h2>Nightly stability (ci/stability.sh 3)</h2><pre>'
        { grep -E '^=== RUN [0-9]+: (PASS|FAIL)' "$s" || true; tail -n 1 "$s"; } | html_escape
        echo '</pre>'
    done
    echo '<h2>Quality gates</h2><pre>'
    if [ -s "$outdir/quality-report.txt" ]; then
        html_escape < "$outdir/quality-report.txt"
    else
        echo '(no quality report: the quality stage did not run)'
    fi
    echo '</pre></body></html>'
} > ci-summary/index.html

count() { # count <metric>: the number in summary.sh's "| <metric> | N |" row
    sed -n "s/^| $1 | \([0-9][0-9]*\) |\$/\1/p" "$summary" | head -n 1
}
coverage=""
if [ -f "$outdir/coverage-summary.txt" ]; then
    coverage=$(awk '$1 == "TOTAL" { print $2 }' "$outdir/coverage-summary.txt")
fi
{
    echo "pass=$(count pass)"
    echo "fail=$(count fail)"
    echo "skip=$(count skip)"
    echo "flaky=$(count flaky)"
    echo "coverage=$coverage"
} > ci-summary/counts.txt
cat ci-summary/counts.txt
