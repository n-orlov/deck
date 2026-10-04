#!/bin/sh
# ci/allure-smoke.sh — smoke check (R196) that the pinned Allure CLI still
# renders the Godog formatter's output: `allure generate` over the formatter's
# fixture results (features/allure_formatter_fixture_test.go, written by
# running that test with DECK_ALLURE_FIXTURE_OUT set), then a look inside the
# generated report. It fails when generation fails, when the fixture
# directory holds no results, or when the report lacks the fixture's steps or
# its `@gh-61` issue link on the "passes with tags" test case.
#
#   ci/allure-smoke.sh <dir holding allure-results/> <report output dir>
#
# Generation goes through ci/allure-report.sh, so it is the same pinned and
# checksum-verified Allure the real report uses (an `allure` on PATH is used
# as it is). Needs python3 for the report check. Runs on the runner host, in
# the `report` job after Java is set up, like ci/allure-report.sh.
set -eu

fixture_dir=${1:?"usage: ci/allure-smoke.sh <dir holding allure-results/> <report output dir>"}
report_dir=${2:?"usage: ci/allure-smoke.sh <dir holding allure-results/> <report output dir>"}
here=$(cd "$(dirname "$0")" && pwd)

have=0
for f in "$fixture_dir"/allure-results/*-result.json; do
    [ -e "$f" ] && have=1
    break
done
if [ "$have" -ne 1 ]; then
    echo "ci/allure-smoke.sh: no *-result.json in $fixture_dir/allure-results; the formatter fixture wrote nothing" >&2
    exit 1
fi

sh "$here/allure-report.sh" "$fixture_dir" "$report_dir"

python3 - "$report_dir" <<'PY'
import glob, json, os, sys

report = sys.argv[1]
cases = []
for path in sorted(glob.glob(os.path.join(report, "data", "test-cases", "*.json"))):
    with open(path) as f:
        cases.append(json.load(f))
if not cases:
    sys.exit("ci/allure-smoke.sh: the generated report has no data/test-cases/*.json")

def steps_of(case):
    stage = case.get("testStage") or {}
    return [s.get("name") for s in stage.get("steps") or []]

tagged = [c for c in cases if c.get("name") == "passes with tags"]
if not tagged:
    sys.exit("ci/allure-smoke.sh: the report has no \"passes with tags\" test case; names: %s" % sorted({c.get("name") for c in cases}))
case = tagged[0]

steps = steps_of(case)
if steps.count("a passing step") != 3:
    sys.exit("ci/allure-smoke.sh: \"passes with tags\" should carry its 3 steps (Background first), report has %r" % steps)

want = "https://github.com/n-orlov/deck/issues/61"
links = [l.get("url") for l in case.get("links") or []]
if want not in links:
    sys.exit("ci/allure-smoke.sh: \"passes with tags\" lacks the @gh-61 link %s, report has %r" % (want, links))

failing = [c for c in cases if c.get("name") == "fails with an attachment"]
if not failing or "the harness fails with a frame" not in steps_of(failing[0]):
    sys.exit("ci/allure-smoke.sh: the failing fixture scenario lacks its failing step in the report")

print("ci/allure-smoke.sh: ok -- %d test cases, steps and the @gh-61 link are in the report" % len(cases))
PY
