#!/bin/sh
# Task 006 (approach 2) — verify every sha and test function this directory's
# README.md cites actually resolves at the commit this script is run from.
#
#   $ sh docs/reports/phase4b-a2-cures/verify-citations.sh
#
# For each of the four findings (B1, B2, B3, R1) the README cites a cure
# commit sha and a test function name. This script enumerates exactly those
# eight facts and checks each one independently:
#
#   - the sha resolves as a commit object:  git cat-file -e <sha>^{commit}
#   - the named function is defined in the named file, at that sha:
#       git show <sha>:<file> | grep '^func <name>('
#
# Exits 0 iff every check passes; exits non-zero and names the first failure
# otherwise. Run from anywhere inside the repo (uses `git rev-parse
# --show-toplevel`).
set -eu

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root"

fail=0

check_commit() {
    label=$1
    sha=$2
    if git cat-file -e "${sha}^{commit}" 2>/dev/null; then
        echo "OK   commit resolves: $label $sha"
    else
        echo "FAIL commit does not resolve: $label $sha"
        fail=1
    fi
}

check_func() {
    label=$1
    sha=$2
    file=$3
    func=$4
    if git show "${sha}:${file}" 2>/dev/null | grep -q "^func ${func}("; then
        echo "OK   func found: $label ${func}( in ${file} at ${sha}"
    else
        echo "FAIL func not found: $label ${func}( in ${file} at ${sha}"
        fail=1
    fi
}

# --- the four findings this README cites -----------------------------------

check_commit "B1" "113b552ca2fe42bb1b3d10e42f14cdc3f735c8e6"
check_func   "B1" "113b552ca2fe42bb1b3d10e42f14cdc3f735c8e6" \
    "internal/tui/filter_default_label_test.go" \
    "TestFilterByDefaultMatchesTheSidebarsOwnDefaultLabel"

check_commit "B2" "075c59c35fbbc22461f748bbe87e87a8ac4139e4"
check_func   "B2" "075c59c35fbbc22461f748bbe87e87a8ac4139e4" \
    "internal/tui/settings_groups_reload_test.go" \
    "TestSettingsGroupsPanelRefreshesOnOrdinaryReload"

check_commit "B3" "00f33a5ac204bc2f558352a18c40fefa24ad3bab"
check_func   "B3" "00f33a5ac204bc2f558352a18c40fefa24ad3bab" \
    "internal/tui/interactive_displacement_scrollcue_test.go" \
    "TestDisplacementFallbackReseedLeavesNoArtificialScrollbackOrCue"

check_commit "R1" "70c7430df3b23a46fb735e8573e26ec55908adeb"
check_func   "R1" "70c7430df3b23a46fb735e8573e26ec55908adeb" \
    "internal/tui/tui_test.go" \
    "TestEmptyAndHelpViewsAreDiscoverable"

if [ "$fail" -ne 0 ]; then
    echo "verify-citations.sh: one or more citations did not resolve" >&2
    exit 1
fi

echo "verify-citations.sh: all eight citations (four commits, four test functions) resolve"
