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

# markdown_html renders the Markdown subset ci/summary.sh writes: headings,
# pipe tables (first row the header), "- " lists, ``` blocks and bare URLs.
# A fail or flaky row with a non-zero count is marked.
markdown_html() {
    awk '
    function esc(s) { gsub(/&/, "\\&amp;", s); gsub(/</, "\\&lt;", s); gsub(/>/, "\\&gt;", s); return s }
    function inline(s) { s = esc(s); gsub(/https?:\/\/[^ <]+/, "<a href=\"&\">&</a>", s); return s }
    function trim(s) { sub(/^[ \t]+/, "", s); sub(/[ \t]+$/, "", s); return s }
    function close_blocks() {
        if (table) { print "</tbody></table>"; table = 0 }
        if (list) { print "</ul>"; list = 0 }
    }
    function cell(tag, s,   v) {
        v = trim(s)
        if (tag == "td" && v ~ /^[0-9]+\.[0-9][0-9][0-9]+$/) v = sprintf("%.1f", v)
        if (v ~ /^[0-9.]+%?$/) return "<" tag " class=\"num\">" esc(v) "</" tag ">"
        return "<" tag ">" inline(v) "</" tag ">"
    }
    code { if ($0 ~ /^```/) { print "</pre>"; code = 0 } else print esc($0); next }
    /^```/ { close_blocks(); printf "<pre>"; code = 1; next }
    /^#+ / {
        close_blocks()
        level = index($0, " ") - 1; if (level < 2) level = 2; if (level > 4) level = 4
        text = $0; sub(/^#+ /, "", text)
        printf "<h%d>%s</h%d>\n", level, inline(text), level
        next
    }
    /^\|[ |-]*$/ { next }
    /^\|/ {
        n = split($0, f, "|")
        if (!table) {
            close_blocks(); print "<table><thead><tr>"
            for (i = 2; i < n; i++) print cell("th", f[i])
            print "</tr></thead><tbody>"; table = 1; next
        }
        cls = ""
        m = trim(f[2]); v = trim(f[3])
        if (m == "fail" && v + 0 > 0) cls = " class=\"bad\""
        if (m == "flaky" && v + 0 > 0) cls = " class=\"warn\""
        printf "<tr%s>", cls
        for (i = 2; i < n; i++) printf "%s", cell("td", f[i])
        print "</tr>"
        next
    }
    /^- / { if (table) close_blocks(); if (!list) { print "<ul>"; list = 1 }; print "<li>" inline(substr($0, 3)) "</li>"; next }
    /^[ \t]*$/ { close_blocks(); next }
    { close_blocks(); print "<p>" inline($0) "</p>" }
    END { close_blocks(); if (code) print "</pre>" }
    '
}

# gates_html renders ci/quality.sh's report: one collapsible section per
# "=== <name> gate ===", open when the gate failed. A gate passed when its
# "what to do:" line says "nothing" (each gate's own pass wording).
gates_html() {
    awk '
    function esc(s) { gsub(/&/, "\\&amp;", s); gsub(/</, "\\&lt;", s); gsub(/>/, "\\&gt;", s); return s }
    function flush(   ok) {
        if (name == "" && body == "") return
        if (name == "") { printf "<pre>%s</pre>\n", body; body = ""; return }
        ok = (body ~ /(^|\n)what to do: nothing/)
        printf "<details%s><summary><span class=\"badge %s\">%s</span> %s</summary><pre>%s</pre></details>\n", \
            ok ? "" : " open", ok ? "ok" : "bad", ok ? "passed" : "failed", esc(name), body
        name = ""; body = ""
    }
    /^=== .* gate ===$/ { flush(); name = $0; sub(/^=== /, "", name); sub(/ ===$/, "", name); next }
    { body = body esc($0) "\n" }
    END { flush() }
    '
}

tile() { # tile <label> <value> <ok|bad|warn|"">
    printf '<div class="tile %s"><div class="v">%s</div><div class="k">%s</div></div>\n' "$3" "${2:--}" "$1"
}

quality_report="$outdir/quality-report.txt"
quality=""
if [ -s "$quality_report" ]; then
    gates=$(grep -c '^=== .* gate ===$' "$quality_report" || true)
    passed=$(grep -c '^what to do: nothing' "$quality_report" || true)
    if [ "${gates:-0}" -gt 0 ] && [ "$gates" = "$passed" ]; then
        quality="passed"
    else
        quality="failed"
    fi
fi

fail=$(count fail)
flaky=$(count flaky)
{
    cat <<'EOF'
<!doctype html><html><head><meta charset="utf-8"><title>deck CI summary</title>
<style>
:root{--fg:#1f2328;--muted:#59636e;--bg:#fff;--panel:#f6f8fa;--line:#d1d9e0;--ok:#1a7f37;--bad:#cf222e;--warn:#9a6700;--link:#0969da}
@media (prefers-color-scheme:dark){:root{--fg:#e6edf3;--muted:#9198a1;--bg:#0d1117;--panel:#151b23;--line:#3d444d;--ok:#3fb950;--bad:#f85149;--warn:#d29922;--link:#4493f8}}
body{font:14px/1.5 system-ui,sans-serif;color:var(--fg);background:var(--bg);margin:1.5em;max-width:72em}
a{color:var(--link)}h1{font-size:1.5em;margin:0 0 .6em}h2{font-size:1.2em;margin:1.6em 0 .5em;padding-bottom:.25em;border-bottom:1px solid var(--line)}h3,h4{font-size:1em;margin:1.2em 0 .4em}
.tiles{display:flex;flex-wrap:wrap;gap:.6em;margin-bottom:1em}
.tile{border:1px solid var(--line);border-radius:6px;padding:.5em .9em;min-width:6em;background:var(--panel)}
.tile .v{font-size:1.4em;font-weight:600;font-variant-numeric:tabular-nums}.tile .k{color:var(--muted);font-size:.85em}
.tile.ok .v{color:var(--ok)}.tile.bad{border-color:var(--bad)}.tile.bad .v{color:var(--bad)}.tile.warn .v{color:var(--warn)}
table{border-collapse:collapse;margin:.4em 0 1em}th,td{border:1px solid var(--line);padding:.25em .7em;text-align:left}th{background:var(--panel)}
.num{text-align:right;font-variant-numeric:tabular-nums}tr.bad td{color:var(--bad);font-weight:600}tr.warn td{color:var(--warn);font-weight:600}
pre{background:var(--panel);border:1px solid var(--line);border-radius:6px;padding:.8em 1em;overflow-x:auto;font-size:12.5px;line-height:1.4}
details{border:1px solid var(--line);border-radius:6px;margin:.5em 0;padding:.3em .8em}details pre{margin:.5em 0}
summary{cursor:pointer;font-weight:600}
.badge{display:inline-block;border-radius:2em;padding:0 .6em;font-size:.8em;font-weight:600;color:#fff}.badge.ok{background:var(--ok)}.badge.bad{background:var(--bad)}
.muted{color:var(--muted)}
</style></head><body>
<h1>deck CI summary</h1>
EOF
    echo '<div class="tiles">'
    tile pass "$(count pass)" ok
    tile fail "$fail" "$([ "${fail:-0}" -gt 0 ] && echo bad || echo ok)"
    tile skip "$(count skip)" ""
    tile flaky "$flaky" "$([ "${flaky:-0}" -gt 0 ] && echo warn)"
    tile coverage "$coverage" ""
    tile quality "$quality" "$(case $quality in passed) echo ok ;; failed) echo bad ;; esac)"
    echo '</div>'
    echo '<h2>Suite</h2>'
    # summary.sh's own title is replaced by the page's.
    grep -v '^### CI suite summary$' "$summary" | markdown_html
    for s in "$outdir"/stability/*/summary.log; do
        [ -f "$s" ] || continue
        echo '<h2>Nightly stability (ci/stability.sh 3)</h2><table><thead><tr><th>run</th><th>result</th></tr></thead><tbody>'
        grep -E '^=== RUN [0-9]+: (PASS|FAIL)' "$s" | sed -E \
            -e 's|^=== RUN ([0-9]+): PASS (.*) ===$|<tr><td class="num">\1</td><td><span class="badge ok">PASS</span> \2</td></tr>|' \
            -e 's|^=== RUN ([0-9]+): FAIL (.*) ===$|<tr><td class="num">\1</td><td><span class="badge bad">FAIL</span> \2</td></tr>|' || true
        echo '</tbody></table>'
        printf '<p>%s</p>\n' "$(tail -n 1 "$s" | html_escape)"
    done
    echo '<h2>Quality gates</h2>'
    if [ -s "$quality_report" ]; then
        gates_html < "$quality_report"
    else
        echo '<p class="muted">(no quality report: the quality stage did not run)</p>'
    fi
    echo '</body></html>'
} > ci-summary/index.html

cat ci-summary/counts.txt
