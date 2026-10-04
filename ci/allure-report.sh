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
# The report is built from ci/suite.sh's allure-results/: features/'s native
# results (grouped `features`, a retried scenario being retries of one test),
# the Go unit tests converted from the merged JUnit (grouped `unit`; every
# retried test's attempts already folded by ci/junitflaky, earlier failures
# becoming hidden retries), and the environment.properties / executor.json
# beside them. A results dir with no allure-results/ (one written before it
# existed) falls back to junit-merged/*.xml, or every top-level *.xml.
set -eu

results_dir=${1:?"usage: ci/allure-report.sh <ci/suite.sh output dir> <report output dir> [<history dir>]"}
report_dir=${2:?"usage: ci/allure-report.sh <ci/suite.sh output dir> <report output dir> [<history dir>]"}
history_dir=${3:-}

# The archive is verified against a pinned SHA-256 before it is unpacked
# (a mismatch aborts; nothing from an unverified archive is ever run). The
# pin below belongs to the default version; overriding ALLURE_VERSION
# requires ALLURE_SHA256 for that version too. ALLURE_URL overrides the
# download location (used by ci/allureverify's test to serve a local file).
ALLURE_VERSION=${ALLURE_VERSION:-2.34.1}
default_allure_sha256=df13c5883429edd5041a24d6d072a8944e65274d1d76a441fd45d2585511349a
if [ "$ALLURE_VERSION" = 2.34.1 ]; then
    ALLURE_SHA256=${ALLURE_SHA256:-$default_allure_sha256}
else
    ALLURE_SHA256=${ALLURE_SHA256:?"ci/allure-report.sh: ALLURE_VERSION=$ALLURE_VERSION has no pinned checksum; set ALLURE_SHA256"}
fi
allure_url=${ALLURE_URL:-"https://github.com/allure-framework/allure2/releases/download/${ALLURE_VERSION}/allure-${ALLURE_VERSION}.tgz"}

sha256_of() { # sha256_of <file>: sha256sum on Linux, shasum on macOS
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | cut -d' ' -f1
    else
        shasum -a 256 "$1" | cut -d' ' -f1
    fi
}

work=$(mktemp -d "${TMPDIR:-/tmp}/deck-allure.XXXXXX")
trap 'rm -rf "$work"' EXIT

allure_bin=$(command -v allure 2>/dev/null || true)
if [ -z "$allure_bin" ]; then
    echo "ci/allure-report.sh: fetching Allure CLI ${ALLURE_VERSION}" >&2
    curl -fsSL "$allure_url" -o "$work/allure.tgz"
    got_sha256=$(sha256_of "$work/allure.tgz")
    if [ "$got_sha256" != "$ALLURE_SHA256" ]; then
        echo "ci/allure-report.sh: checksum mismatch for the Allure ${ALLURE_VERSION} archive: want ${ALLURE_SHA256}, got ${got_sha256}; aborting" >&2
        exit 1
    fi
    mkdir -p "$work/allure"
    tar -xzf "$work/allure.tgz" -C "$work/allure" --strip-components=1
    allure_bin="$work/allure/bin/allure"
fi
"$allure_bin" --version >&2

allure_results="$work/allure-results"
mkdir -p "$allure_results"

# Native results first (task 005, R195/R196): ci/suite.sh writes one
# allure-results/ holding features/'s native results (parentSuite "features"),
# the Go unit tests converted from the merged JUnit (parentSuite "unit"),
# environment.properties and executor.json, so it is copied as it is. Only a
# results dir without one (written before it existed) falls back to the
# merged JUnit files, as before.
native_dir="$results_dir/allure-results"
have_native=0
for f in "$native_dir"/*-result.json; do
    [ -e "$f" ] && have_native=1
    break
done
if [ "$have_native" -eq 1 ]; then
    for f in "$native_dir"/*; do
        [ -f "$f" ] || continue
        case "$f" in */godog-allure-summary.txt) continue ;; esac
        cp "$f" "$allure_results/"
    done
else
    junit_dir="$results_dir/junit-merged"
    if [ ! -d "$junit_dir" ]; then
        echo "ci/allure-report.sh: no $native_dir and no $junit_dir, reading the raw per-attempt JUnit files" >&2
        junit_dir=$results_dir
    else
        echo "ci/allure-report.sh: no native results in $native_dir, reading $junit_dir" >&2
    fi
    for f in "$junit_dir"/*.xml; do
        [ -e "$f" ] || continue
        cp "$f" "$allure_results/"
    done
fi

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
