#!/bin/sh
# ci/quality.sh -- R187's one quality-gate entry point. It scrubs every
# `DECK_*` variable from its own environment (the same leak this repo's
# other scripts guard against -- see ci/suite.sh's own header), then runs
# `go run ./ci/qualitycheck` against the single checked-in thresholds
# file, ci/quality.json, and the merged coverage profile ci/suite.sh
# produces. ci/qualitycheck does the actual gate work (reading
# thresholds, running whichever gates are switched on, formatting one
# report section per on gate); this script's only job is environment
# hygiene and resolving the two paths qualitycheck needs.
#
#   ci/run.sh ci/quality.sh                  # suite-outdir defaults to ci-results
#   ci/run.sh ci/quality.sh ci-results        # same, explicit
#   ci/run.sh ci/quality.sh /tmp/out my.json  # a test's own outdir + config
#
# Every gate in ci/quality.json is flipped on only after the product passes
# it locally; the golangci gate (R188) runs ci/golangci.sh, the same single
# invocation ci/lint.sh runs. With every gate off this script still runs, prints that
# nothing is enabled, and exits 0 -- so it is safe to wire into ci.yml
# (task 086) before any gate is live.
set -eu

repo_root=$(cd "$(dirname "$0")/.." && pwd)
cd "$repo_root"

# outdir: the directory ci/suite.sh wrote its merged coverage profile
# into (coverage-merged.out). Positional, not DECK_CI_OUT, so a caller
# never needs a DECK_* variable just to point this script somewhere --
# consistent with task 002's own outdir argument on ci/suite.sh.
outdir=${1:-ci-results}

# config: the thresholds file. Positional and optional so a test can
# point this script at its own seeded copy without touching the
# checked-in ci/quality.json (which never changes for a test run).
config=${2:-ci/quality.json}

# Scrub leaked DECK_* environment (R187: "Gate and suite scripts scrub
# every DECK_* variable"), identical in mechanism to ci/suite.sh's own
# scrub -- a stray DECK_GODOG_PATHS/DECK_HOME from the caller's shell
# must never reach `go run ./ci/qualitycheck` or anything it spawns
# (ci/crapgate, in turn, via `go run ./ci/crapgate`).
for deck_var in $(env | sed -n 's/^\(DECK_[A-Za-z_][A-Za-z0-9_]*\)=.*/\1/p' | sort -u); do
    unset "$deck_var"
done

profile="$outdir/coverage-merged.out"

# Trivy's DB cache lives under the image's /go-cache volume so a warm run
# is fast (R190); outside the image (no /go-cache) it falls back to a
# temp dir. TRIVY_CACHE_DIR overrides both.
if [ -n "${TRIVY_CACHE_DIR:-}" ]; then
    trivy_cache=$TRIVY_CACHE_DIR
elif [ -d /go-cache ] && [ -w /go-cache ]; then
    trivy_cache=/go-cache/trivy
else
    trivy_cache=${TMPDIR:-/tmp}/deck-trivy-cache
fi

# govulncheck's vuln-DB cache goes to the same volume (it follows
# XDG_CACHE_HOME); outside the image it inherits the caller's own cache.
govuln_cache=
if [ -d /go-cache ] && [ -w /go-cache ]; then
    govuln_cache=/go-cache/xdg-cache
    mkdir -p "$govuln_cache"
fi

# R187: the gate report is also written to "$outdir/quality-report.txt", so
# it crosses the ci/run.sh sibling boundary on the bind-mounted workspace and
# the runner-side `Quality gate report` step in ci.yml can append it to
# GITHUB_STEP_SUMMARY whether the gates pass or fail. stdout (what a local
# run prints) is the same text; only stdout is captured, so go run's and
# qualitycheck's stderr (usage/config errors) stays on stderr.
report_file="$outdir/quality-report.txt"
mkdir -p "$outdir"
rm -f "$report_file"

exit_code=0
go run ./ci/qualitycheck -config "$config" -profile "$profile" \
    -trivy-target . -trivy-cache "$trivy_cache" -trivy-ignore .trivyignore \
    -govulncheck-target . -govulncheck-cache "$govuln_cache" \
    > "$report_file" || exit_code=$?

# A run that died before printing any gate section (bad config, build
# failure) still leaves a report saying so, never an empty or missing file.
if [ ! -s "$report_file" ]; then
    echo "ci/quality.sh: no gate report was produced (exit status $exit_code); see the step log for the error." > "$report_file"
fi

cat "$report_file"
exit "$exit_code"
