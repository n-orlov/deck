#!/bin/sh
# ci/allure-report.sh — builds an Allure report (R146) from one
# ci/suite.sh output directory. Not run through ci/run.sh: the Allure
# commandline distribution needs a JRE, which ci/Dockerfile (protected from
# ralphd jobs) does not carry, so this runs directly on the CI runner host
# (workflow steps: actions/setup-java first, then this script), exactly
# like ci/stability.sh does for the same reason. Local, ad-hoc use works
# too, given a JRE on PATH.
#
#   ci/allure-report.sh <ci/suite.sh output dir> <report output dir> [<history dir>]
#
# <history dir>, when given and non-empty, is copied into the Allure
# results directory as `history/*.json` before `allure generate` runs, so
# the previous published report's trend/retries data carries into the new
# one (Allure's own convention: a results dir's `history/` subfolder is
# read back by `generate` and re-emitted, unchanged in shape, into the new
# report's own `history/`). Omit it (or point it at an empty/missing dir)
# for a report with no carried history, e.g. a PR run's `/pr/<n>/` report,
# which the design deliberately keeps unlinked to any shared trend.
#
# Every *.xml file already in the results dir that looks like a JUnit
# report is picked up: Allure's junit-xml plugin sniffs file *content*
# (a <testsuite>/<testsuites> root), not the file name, so ci/suite.sh's
# junit-go.xml, junit-features.xml and any junit-features-rerun-*.xml are
# all read as-is, with no reformatting step of our own.
set -eu

results_dir=${1:?"usage: ci/allure-report.sh <ci/suite.sh output dir> <report output dir> [<history dir>]"}
report_dir=${2:?"usage: ci/allure-report.sh <ci/suite.sh output dir> <report output dir> [<history dir>]"}
history_dir=${3:-}

ALLURE_VERSION=${ALLURE_VERSION:-2.34.1}
allure_url="https://github.com/allure-framework/allure2/releases/download/${ALLURE_VERSION}/allure-${ALLURE_VERSION}.tgz"

work=$(mktemp -d "${TMPDIR:-/tmp}/deck-allure.XXXXXX")
trap 'rm -rf "$work"' EXIT

allure_bin=$(command -v allure 2>/dev/null || true)
if [ -z "$allure_bin" ]; then
    echo "ci/allure-report.sh: fetching Allure CLI ${ALLURE_VERSION}" >&2
    curl -fsSL "$allure_url" -o "$work/allure.tgz"
    mkdir -p "$work/allure"
    tar -xzf "$work/allure.tgz" -C "$work/allure" --strip-components=1
    allure_bin="$work/allure/bin/allure"
fi
"$allure_bin" --version >&2

allure_results="$work/allure-results"
mkdir -p "$allure_results"

for f in "$results_dir"/*.xml; do
    [ -e "$f" ] || continue
    cp "$f" "$allure_results/"
done

if [ -n "$history_dir" ] && [ -d "$history_dir" ]; then
    have_history=0
    for f in "$history_dir"/*.json; do
        [ -e "$f" ] || continue
        have_history=1
        mkdir -p "$allure_results/history"
        cp "$f" "$allure_results/history/"
    done
    if [ "$have_history" -eq 1 ]; then
        echo "ci/allure-report.sh: carried history from $history_dir" >&2
    else
        echo "ci/allure-report.sh: $history_dir has no history/*.json, starting fresh" >&2
    fi
else
    echo "ci/allure-report.sh: no history dir given, this report starts fresh" >&2
fi

rm -rf "$report_dir"
"$allure_bin" generate "$allure_results" --clean -o "$report_dir"
echo "ci/allure-report.sh: report written to $report_dir"
