#!/bin/sh
# ci/suite.sh — the CI suite runner (R145): drives the whole test matrix
# (`go test -p=1 -count=1 ./...`: pass 1 is every test function in every
# package except features/TestFeatures, pass 2 is TestFeatures) through
# gotestsum, fetched at a
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
# features/ is different in exactly one test: TestFeatures, the single Go
# test function that fans out into 300+ Godog scenarios via
# godog.TestSuite. Handing it to gotestsum's rerun-fails would rerun
# `-run '^TestFeatures$'` on a failure, which re-executes every scenario in
# the package again — exactly the "whole package" rerun task 015's
# criterion (3) forbids. So the gotestsum pass covers EVERY package,
# features/ included, with `-skip '^TestFeatures$'`: features/'s ordinary
# Go test functions (probes, harness self-tests, ...) run and are retried
# there like any other test. TestFeatures alone is then run once, also
# through gotestsum but without --rerun-fails, in a second pass; on
# failure this script parses the pretty formatter's own "--- Failed
# steps:" summary (always emitted, in both the terminal and the
# JUnit-carrying "pretty,junit:<path>" format — see
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
# Canonicalize to an absolute path now, once. features/TestFeatures's own
# test binary runs with its CWD set to the features/ package directory (an
# ordinary `go test` behaviour, not something this script controls), which
# is NOT $repo_root -- so a relative DECK_CI_OUT (e.g. the CI workflow's
# "ci-results", chosen so the *host* steps outside the sibling container can
# use it relative to their own checkout) would make that binary's own
# DECK_GODOG_JUNIT open fail with "no such file or directory" even though
# the exact same relative path is valid from repo_root. Every use of
# $outdir below (unit_coverprofile, go_junit, features_junit, covdir, ...)
# is now absolute, so it resolves the same regardless of which process's
# CWD interprets it.
outdir=$(cd "$outdir" && pwd)
echo "ci/suite.sh: writing JUnit/flaky/log files to $outdir"

overall_status=0

# --- 1. every Go test except TestFeatures, via gotestsum ---
# DECK_CI_GO_PACKAGES lets a caller narrow the package list (used by this
# task's own local demonstration, so a scratch worktree's throwaway package
# can be exercised without also re-running the whole real suite); unset,
# the default is every package (./...), features/ included. The
# `-skip '^TestFeatures$'` below removes only the Godog entry point, which
# pass 2 owns; every other test function in features/ runs here.
pkgs=${DECK_CI_GO_PACKAGES:-$(go list ./...)}

# DECK_CI_GO_EXTRA_FLAGS (task 017, R145 nightly): extra `go test` flags
# appended, unquoted on purpose, to BOTH the unit pass below and the
# features/TestFeatures pass further down -- the nightly-only `-race` flag
# is the reason this exists, but it is a generic pass-through, not a
# race-specific hook. Unset (the default, every PR/push/dispatch run),
# this expands to nothing and both passes run exactly as before.
#
# `-covermode=atomic` is mandatory once `-race` is enabled (`go test`
# refuses `set`/`count` alongside it), so the unit pass's covermode tracks
# whether the extra flags mention -race rather than hardcoding `set`; every
# other caller (extra flags unset) keeps the original `set` mode.
covermode=set
case " ${DECK_CI_GO_EXTRA_FLAGS:-} " in
    *' -race '*) covermode=atomic ;;
esac

go_junit="$outdir/junit-go.xml"
go_rerun_report="$outdir/rerun-report-go.txt"
go_flaky="$outdir/flaky-go.txt"
: > "$go_flaky"

# unit_coverprofile (R146/task 016 criterion 1, R146 cure-01-05): a
# legacy-format coverage profile covering every package this pass tests.
# task 016 originally produced this with the *ordinary* `go test
# -coverprofile` flag, fed straight into gotestsum's --packages/-- args --
# but gotestsum's own --rerun-fails=1 reruns a failed test with a SECOND,
# separate `go test -run '^Name$' <pkg> ... -coverprofile=<same path>`
# invocation, and each `go test` invocation, retry included, WRITES that
# path from scratch rather than merging into whatever is already there. So
# the moment any test anywhere in the pass needed a retry, that retry's own
# `go test` call clobbered the whole profile with only its own (one
# package, one test) coverage -- deleting every other tested package
# outright and zeroing every block the retried package's OTHER, passing
# tests had covered on the first attempt (cure-01-05, reviewer probe
# coverage-retention-probe.log: StableCovered count 0, RetryCovered count
# 1, ci/releasegate's whole profile gone).
#
# The fix is to stop asking `go test` to write the legacy format directly at
# all, and instead use the same retry-safe, per-process-counter-file
# mechanism already proven correct for features/'s black-box coverage
# below -- but reached through `-test.gocoverdir`, the internal test-binary
# flag `-coverprofile` itself sets under the hood (Go 1.20+; see `go help
# testflag`'s -cover/-coverprofile entries and cmd/internal/test2json), not
# through the GOCOVERDIR *environment variable*: that env var only steers
# coverage for a separately built+run binary (go build -cover, as
# features/ does with the deck binary itself); an ordinary `go test`
# invocation ignores it and, absent -coverprofile, writes no profile at
# all -- confirmed empirically before settling on -args -test.gocoverdir,
# see /run/ralphd/artifacts/cure-01-05/. Passed via `-args` (everything
# after -args goes to the test binary, not to `go test` itself), every `go
# test -cover` invocation (the initial full pass AND each gotestsum retry)
# writes its OWN uniquely-named counter file set into unit_covdir rather
# than overwriting a shared path, so counters from every attempt --
# including packages/tests that were never retried at all -- coexist and
# accumulate instead of clobbering each other. `go tool covdata textfmt`
# below then bridges that whole directory into the one legacy-format
# unit_coverprofile file the rest of this script (and coverage_table)
# already expects, so no downstream consumer needs to know the collection
# mechanism changed.
unit_covdir="$outdir/covdata-unit"
mkdir -p "$unit_covdir"
unit_coverprofile="$outdir/coverage-unit.out"

if [ -n "$pkgs" ]; then
    go_status=0
    go run "$gotestsum_pkg" \
        --format testname \
        --junitfile "$go_junit" \
        --rerun-fails=1 \
        --rerun-fails-report "$go_rerun_report" \
        --packages "$pkgs" \
        -- -p=1 -count=1 -skip '^TestFeatures$' "-covermode=$covermode" -cover ${DECK_CI_GO_EXTRA_FLAGS:-} -args "-test.gocoverdir=$unit_covdir" \
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
    echo "ci/suite.sh: no Go packages found (go list ./... returned nothing)" >&2
    overall_status=1
fi

# --- 2. features/TestFeatures: one run, then a per-scenario rerun of anything failed ---
features_junit="$outdir/junit-features.xml"
features_log="$outdir/features-run1.log"
features_flaky="$outdir/flaky-features.txt"
: > "$features_flaky"

# covdir (R146/task 016, criterion 2): a GOCOVERDIR passed to every
# features/TestFeatures invocation below (the initial run and every solo
# scenario rerun). features/lifecycle_test.go's registerScenarioLifecycle
# Before hook reads this same env var from its own process's environment
# (inherited automatically by the deck binaries it spawns, since
# StartScreenDriverInDir's cmd.Env starts from os.Environ()) and adds
# `go build -cover` only when it is non-empty, so every scenario's deck
# process writes black-box coverage counters here. Each rebuilt binary and
# each spawned process gets its own uniquely-named counter file, so reusing
# one directory across the whole pass (including reruns) just accumulates
# more data rather than clobbering it.
covdir="$outdir/covdata-features"
mkdir -p "$covdir"

# run_test_features <gotestsum-junit-path> <NAME=value>...: one gotestsum-driven
# run of features/TestFeatures and nothing else. No --rerun-fails here on purpose
# (see the header); --format standard-verbose keeps Godog's pretty output,
# including its "--- Failed steps:" summary, verbatim in the log. Every
# NAME=value goes through env(1) rather than a prefix assignment on the
# function call, whose export to child processes POSIX leaves unspecified.
run_test_features() {
    junit=$1
    shift
    env "$@" go run "$gotestsum_pkg" \
        --format standard-verbose \
        --junitfile "$junit" \
        --packages ./features/ \
        -- -p=1 -count=1 -run '^TestFeatures$' ${DECK_CI_GO_EXTRA_FLAGS:-}
}

features_status=0
if ! run_test_features "$outdir/junit-features-gotestsum.xml" "DECK_GODOG_JUNIT=$features_junit" "GOCOVERDIR=$covdir" > "$features_log" 2>&1; then
    features_status=1
    clean_log="$outdir/features-run1.clean.log"
    sed -E 's/\x1b\[[0-9;]*m//g' "$features_log" > "$clean_log"

    # Only Godog's own summary block ("--- Failed steps:" up to its
    # "N scenarios (...)" tally) names failed scenarios; bounding the range
    # keeps any later reprint of passing scenarios' headers (gotestsum's
    # "=== Failed" recap) from being mistaken for a failure. The recap can
    # repeat the summary block itself, hence sort -u.
    locations=$(sed -n '/^--- Failed steps:/,/^[0-9][0-9]* scenarios (/p' "$clean_log" \
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
            if run_test_features "$outdir/junit-features-rerun-$i-gotestsum.xml" "DECK_GODOG_PATHS=$loc" "DECK_GODOG_JUNIT=$outdir/junit-features-rerun-$i.xml" "GOCOVERDIR=$covdir" > "$rerun_log" 2>&1; then
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

# --- 3. one merged JUnit file per pass, retries folded (R146/task 019) ---
# The raw files above keep every attempt as its own <testcase>, and
# Allure's junit-xml plugin, given those, can pick a test's failed attempt
# as its result and report a test that passed on retry as failed and not
# flaky. ci/junitflaky (see its own header) folds each test whose last
# attempt passed into one passing <testcase> carrying its earlier failures
# as <rerunFailure>/<rerunError>, the plugin's own flaky convention, and
# recounts. junit-merged/ is what ci/allure-report.sh and ci/summary.sh
# read. features/ contributes Godog's own per-scenario JUnit (the original
# pass, then each solo rerun, in rerun order); gotestsum's view of the same
# pass (TestFeatures/<scenario> subtests) is used instead only if Godog's
# file is missing, e.g. when TestFeatures never got as far as running a
# scenario. A merge failure is reported, not fatal: the report then shows
# only what did merge.
merged_dir="$outdir/junit-merged"
mkdir -p "$merged_dir"

# merge_junit <output> <input>...: the inputs in chronological order; any
# that does not exist is left out.
merge_junit() {
    merged_out=$1
    shift
    merge_inputs=""
    for f in "$@"; do
        [ -f "$f" ] && merge_inputs="$merge_inputs $f"
    done
    [ -n "$merge_inputs" ] || return 0
    # shellcheck disable=SC2086 # $outdir is canonical, one word per path
    if ! go run ./ci/junitflaky -o "$merged_out" $merge_inputs; then
        echo "ci/suite.sh: could not merge$merge_inputs into $merged_out" >&2
        rm -f "$merged_out"
    fi
}

merge_junit "$merged_dir/junit-go.xml" "$go_junit"
# Rerun files are numbered 1..i in the order the reruns ran.
features_reruns=""
features_reruns_gotestsum=""
n=1
while [ -f "$outdir/junit-features-rerun-$n-gotestsum.xml" ] || [ -f "$outdir/junit-features-rerun-$n.xml" ]; do
    features_reruns="$features_reruns $outdir/junit-features-rerun-$n.xml"
    features_reruns_gotestsum="$features_reruns_gotestsum $outdir/junit-features-rerun-$n-gotestsum.xml"
    n=$((n + 1))
done
if [ -f "$features_junit" ]; then
    # shellcheck disable=SC2086
    merge_junit "$merged_dir/junit-features.xml" "$features_junit" $features_reruns
else
    # shellcheck disable=SC2086
    merge_junit "$merged_dir/junit-features.xml" "$outdir/junit-features-gotestsum.xml" $features_reruns_gotestsum
fi

# --- 4. coverage summary (R146/task 016, R146 cure-01-05) ---
# `unit_coverprofile` and `features_coverprofile` are both bridged the same
# way now: `unit_covdir`/`covdir` hold the GOCOVERDIR binary format, one
# counter file per `go test`/deck process invocation (every gotestsum
# retry, and every features/ scenario's spawned deck binary, gets its own
# uniquely-named files rather than overwriting a shared one, which is
# exactly what keeps a retry from deleting anything the earlier attempt
# measured -- see the cure-01-05 comment above unit_covdir). `go tool
# covdata textfmt` bridges each directory into the same legacy text format
# `go tool cover` understands, so the one small awk aggregator below can
# read both without needing two unrelated summarizers.
#
# A covdir with no data (every process killed before its coverage atexit
# hook ran, or nothing ran at all) makes `textfmt` print a warning but
# still EXIT 0, writing a completely empty (zero-byte, no "mode:" header)
# file rather than failing -- so the fallback below cannot rely on the
# command's own exit status; it checks the resulting file is actually
# non-empty instead (cure-01-05: an empty section confuses the awk
# aggregator's FNR==1 file-boundary detection just as much as a missing
# one would, attributing the NEXT profile's own "mode:" header line to the
# wrong section and silently swapping its data into the other column --
# caught while proving this fix, /run/ralphd/artifacts/cure-01-05/).
ensure_legacy_profile() {
    # ensure_legacy_profile <path>: guarantee <path> exists and has at
    # least the "mode: <mode>" header line every legacy coverage profile
    # needs, regardless of why it might otherwise be missing or empty.
    profile_path=$1
    if [ ! -s "$profile_path" ]; then
        printf 'mode: set\n' > "$profile_path"
    fi
}

features_coverprofile="$outdir/coverage-features.out"
if ! go tool covdata textfmt -i="$covdir" -o="$features_coverprofile" 2>"$outdir/covdata-textfmt.log"; then
    echo "ci/suite.sh: no features/ black-box coverage data in $covdir (see $outdir/covdata-textfmt.log)" >&2
fi
ensure_legacy_profile "$features_coverprofile"
if ! go tool covdata textfmt -i="$unit_covdir" -o="$unit_coverprofile" 2>"$outdir/covdata-unit-textfmt.log"; then
    echo "ci/suite.sh: no unit coverage data in $unit_covdir (see $outdir/covdata-unit-textfmt.log)" >&2
fi
ensure_legacy_profile "$unit_coverprofile"

# coverage_table <unit-profile> <features-profile>: prints one
# package-by-package table (plus a TOTAL row) with a column for each
# profile's statement coverage. Both profiles share the same legacy format
# (`mode: <mode>` then `<import/path/file.go>:<pos> <numstmt> <count>` lines),
# so a package name is just the line's file field with its own filename
# dropped. A package present in only one profile prints "-" in the other
# column rather than 0.0%, so "not exercised by this pass" stays visually
# distinct from "exercised, zero coverage".
coverage_table() {
    awk '
    function pkgof(f) {
        sub(/:.*/, "", f)
        n = split(f, parts, "/")
        pkg = parts[1]
        for (i = 2; i < n; i++) pkg = pkg "/" parts[i]
        return pkg
    }
    FNR == 1 { section++; next }
    {
        pkg = pkgof($1)
        numstmt = $2 + 0
        count = $3 + 0
        seen[pkg] = 1
        if (section == 1) {
            u_total[pkg] += numstmt
            if (count > 0) u_cov[pkg] += numstmt
            gu_total += numstmt
            if (count > 0) gu_cov += numstmt
        } else {
            f_total[pkg] += numstmt
            if (count > 0) f_cov[pkg] += numstmt
            gf_total += numstmt
            if (count > 0) gf_cov += numstmt
        }
    }
    END {
        printf "%-55s %10s %18s\n", "package", "unit", "features/ (black-box)"
        for (p in seen) {
            ustr = (p in u_total) ? sprintf("%.1f%%", (u_total[p] > 0 ? u_cov[p] * 100.0 / u_total[p] : 0)) : "-"
            fstr = (p in f_total) ? sprintf("%.1f%%", (f_total[p] > 0 ? f_cov[p] * 100.0 / f_total[p] : 0)) : "-"
            printf "%-55s %10s %18s\n", p, ustr, fstr
        }
        ut = (gu_total > 0) ? sprintf("%.1f%%", gu_cov * 100.0 / gu_total) : "0.0%"
        ft = (gf_total > 0) ? sprintf("%.1f%%", gf_cov * 100.0 / gf_total) : "0.0%"
        printf "%-55s %10s %18s\n", "TOTAL", ut, ft
    }' "$1" "$2"
}

coverage_summary="$outdir/coverage-summary.txt"
coverage_table "$unit_coverprofile" "$features_coverprofile" | tee "$coverage_summary"

echo "ci/suite.sh: done (go_flaky=$go_flaky, features_flaky=$features_flaky, outdir=$outdir, coverage_summary=$coverage_summary)"
exit "$overall_status"
