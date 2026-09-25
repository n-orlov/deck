#!/bin/sh
# ci/allure-site.sh — merges one freshly generated Allure report into the
# persisted Pages site tree (R146): a main/nightly report replaces the
# site root (leaving any live pr/<n>/ subtree alone), a PR report replaces
# only its own pr/<n>/ subtree (leaving the root and every other PR alone).
# This is what lets a single `actions/deploy-pages` artifact -- which
# always replaces the *whole* site -- carry every still-live report
# forward, run after run, even though any one CI run only regenerates the
# report it is itself responsible for.
#
#   ci/allure-site.sh <site dir> root
#   ci/allure-site.sh <site dir> pr <number>
#
# <site dir> must already hold the previous site tree (the caller's job:
# fetch it, e.g. from a persisted branch, or start an empty dir on a first
# run) plus the freshly generated report staged at <site dir>/.new-report.
# This script only owns *where in the tree* that staged report lands
# relative to what is already there; it never touches history/trend data,
# which is entirely ci/allure-report.sh's own concern one step earlier.
set -eu

site=${1:?"usage: ci/allure-site.sh <site dir> root|pr [<number>]"}
mode=${2:?"usage: ci/allure-site.sh <site dir> root|pr [<number>]"}

staged="$site/.new-report"
[ -d "$staged" ] || { echo "ci/allure-site.sh: $staged missing (nothing staged)" >&2; exit 1; }

case "$mode" in
    root)
        dest="$site"
        find "$site" -mindepth 1 -maxdepth 1 ! -name pr ! -name .new-report -exec rm -rf {} +
        ;;
    pr)
        number=${3:?"usage: ci/allure-site.sh <site dir> pr <number>"}
        case "$number" in
            ''|*[!0-9]*) echo "ci/allure-site.sh: <number> must be numeric, got '$number'" >&2; exit 1 ;;
        esac
        dest="$site/pr/$number"
        rm -rf "$dest"
        mkdir -p "$dest"
        ;;
    *)
        echo "ci/allure-site.sh: unknown mode '$mode' (want root or pr)" >&2
        exit 1
        ;;
esac

cp -a "$staged/." "$dest/"
rm -rf "$staged"
echo "ci/allure-site.sh: merged into $dest"
