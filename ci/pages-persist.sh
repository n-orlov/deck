#!/bin/sh
# ci/pages-persist.sh — R165 (GH #50): commits and pushes the merged Allure
# site tree to gh-pages, with a bounded retry when the push is rejected
# because another run's report landed on gh-pages first (a
# non-fast-forward push -- two `report` jobs finishing around the same
# time is expected: a push to main and its own PR preview, or two PRs,
# routinely race). ci.yml's "report" job used to do this itself as a
# single unretried `git push`; this script is what it now delegates to,
# so the retry logic has one place to be tested (see ci/pagespersist/).
#
# Each retry re-fetches gh-pages, resets the local checkout to the newer
# remote tip, and re-runs ci/allure-site.sh to re-merge this run's own
# freshly rendered report on top of that newer tree, before trying to
# push again -- so whichever of a root report and a concurrent pr/<n>/
# report loses the race just retries forward, and neither clobbers the
# other's contribution once both have landed.
#
#   ci/pages-persist.sh <site dir> <report dir> root [<max attempts>]
#   ci/pages-persist.sh <site dir> <report dir> pr <number> [<max attempts>]
#
# <site dir> must already be a git checkout of gh-pages (or a freshly
# orphaned one, on a first run), with `origin` set up and authenticated --
# exactly the state ci.yml's "Check out gh-pages, or start it as an
# orphan" step leaves it in.
#
# <report dir> is the freshly rendered Allure report, untouched by this
# script. ci/allure-site.sh only ever consumes a staged copy at
# <site dir>/.new-report (and deletes it once merged), so this script
# re-stages a fresh copy from <report dir> on every attempt, which is what
# lets the same rendered report be re-merged after a retry's re-fetch.
set -eu

usage() {
    echo "usage: ci/pages-persist.sh <site dir> <report dir> root|pr [<number>] [<max attempts>]" >&2
    exit 1
}

[ $# -ge 3 ] || usage
site=$1
report=$2
mode=$3
shift 3

number=""
case "$mode" in
    root)
        ;;
    pr)
        [ $# -ge 1 ] || usage
        number=$1
        shift
        case "$number" in
            ''|*[!0-9]*) echo "ci/pages-persist.sh: <number> must be numeric, got '$number'" >&2; exit 1 ;;
        esac
        ;;
    *)
        usage
        ;;
esac

max_attempts=${1:-5}
case "$max_attempts" in
    ''|*[!0-9]*) echo "ci/pages-persist.sh: <max attempts> must be numeric, got '$max_attempts'" >&2; exit 1 ;;
esac
[ "$max_attempts" -ge 1 ] || { echo "ci/pages-persist.sh: <max attempts> must be >= 1" >&2; exit 1; }

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

merge_report() {
    rm -rf "$site/.new-report"
    mkdir -p "$site/.new-report"
    cp -a "$report/." "$site/.new-report/"
    if [ "$mode" = "root" ]; then
        "$script_dir/allure-site.sh" "$site" root
    else
        "$script_dir/allure-site.sh" "$site" pr "$number"
    fi
}

attempt=1
while :; do
    merge_report

    if (cd "$site" && git add -A && git diff --cached --quiet); then
        echo "ci/pages-persist.sh: no site changes to persist"
        exit 0
    fi

    (
        cd "$site"
        git -c user.name="github-actions[bot]" \
            -c user.email="41898282+github-actions[bot]@users.noreply.github.com" \
            commit -q -m "allure: $mode report for run ${DECK_PAGES_RUN_ID:-$$}"
    )

    if (cd "$site" && git push -q origin gh-pages); then
        echo "ci/pages-persist.sh: pushed on attempt $attempt"
        exit 0
    fi

    if [ "$attempt" -ge "$max_attempts" ]; then
        echo "ci/pages-persist.sh: push rejected after $attempt attempt(s), giving up" >&2
        exit 1
    fi

    echo "ci/pages-persist.sh: push rejected (attempt $attempt of $max_attempts), re-fetching gh-pages and retrying" >&2
    (
        cd "$site"
        git fetch --quiet origin gh-pages
        git checkout -q -B gh-pages origin/gh-pages
    )
    attempt=$((attempt + 1))
done
