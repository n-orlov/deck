#!/usr/bin/env bash
# Task 110 (approach 02) — citation checker for the rows this task added or edited
# in docs/reports/phase3g-findings.md.
#
# Scope: the *added* lines of commit 961e9cc's diff of docs/reports/phase3g-findings.md
# plus this follow-up commit's own additions, i.e. F18-F21's resolutions, the new
# F24-F26 rows and the §6 addendum. Every backticked sha, every markdown link and
# every backticked in-tree path in those lines is checked here — nothing is sampled.
#
# Run from the repo root:  bash docs/reports/phase3g-110-findings-update/check-citations.sh
# Exit status 0 means every citation resolved.

set -u
cd "$(git rev-parse --show-toplevel)" || exit 2
fail=0

echo "== HEAD =="
git rev-parse --short HEAD

echo
echo "== shas cited in the new/edited rows (git cat-file -e) =="
for sha in 0219e42 045a6e6 050ca9f 15e33c6 1c8cbad 1cfbd5a 2549406 51b7f17 57a6882 \
           7033e12 89682e5 89fcffc 8cff03b 9991689 b6cbbc7 ea6ce4b fe79040 961e9cc; do
  if git cat-file -e "$sha^{commit}" 2>/dev/null; then
    printf '%s ok  %s\n' "$sha" "$(git log -1 --format=%s "$sha")"
  else
    printf '%s MISSING\n' "$sha"; fail=1
  fi
done

echo
echo "== markdown links in the new/edited rows (relative to docs/reports/) =="
for rel in phase3g-101-agent-wait-wrap/README.md \
           phase3g-102-clear-recents-label/README.md \
           phase3g-103-hook-sessionend-repair/README.md \
           phase3g-104-title-independent-waits/README.md \
           phase3g-105-rename-selection/README.md \
           phase3g-106-contrast-floor/README.md \
           phase3g-110-findings-update/citation-check.log \
           phase3g-110-findings-update/check-citations.sh \
           phase3g-030-reclaim-leaked-interactive-pipe/README.md; do
  if test -e "docs/reports/$rel"; then
    printf 'docs/reports/%s ok\n' "$rel"
  else
    printf 'docs/reports/%s MISSING\n' "$rel"; fail=1
  fi
done

echo
echo "== same-document anchors: every '](#...)' link in the report, GitHub slug rule =="
python3 - <<'PY' || fail=1
import re, sys
text = open('docs/reports/phase3g-findings.md').read()

def slug(t):
    t = t.replace('`', '')
    t = re.sub(r'[^\w\- ]', '', t.lower(), flags=re.UNICODE)
    return t.replace(' ', '-')

headings = {slug(m.group(1)) for m in re.finditer(r'(?m)^#+ (.*)$', text)}
bad = 0
for a in dict.fromkeys(re.findall(r'\]\((#[^)]+)\)', text)):
    ok = a[1:] in headings
    print('%s %s' % (a, 'ok (heading present)' if ok else 'MISSING heading'))
    bad += 0 if ok else 1
sys.exit(1 if bad else 0)
PY

echo
echo "== in-tree paths cited in the new/edited rows =="
for f in ci/run.sh \
         features/create_session.feature \
         features/durable_identity.feature \
         features/event_log.feature \
         features/kill_delete_undo.feature \
         features/settings.feature \
         features/status_claude_hooks.feature \
         features/status_recovery.feature \
         features/agent_steps_test.go \
         internal/service/reconcile.go \
         internal/theme/contrast_test.go \
         internal/tui/rename.go \
         internal/tui/rename_theme_test.go \
         docs/reports/phase3g-002-r76-field-route/README.md; do
  if test -e "$f"; then printf '%s ok\n' "$f"; else printf '%s MISSING\n' "$f"; fail=1; fi
done

echo
echo "== SPEC.md untouched by this task's commits =="
git log --oneline 961e9cc~1..HEAD -- SPEC.md | sed 's/^/  /'
if [ -z "$(git log --format=%h 961e9cc~1..HEAD -- SPEC.md)" ]; then
  echo "SPEC.md ok (no commit in 961e9cc~1..HEAD touches it)"
else
  echo "SPEC.md MODIFIED"; fail=1
fi

echo
if [ "$fail" -eq 0 ]; then echo "ALL CITATIONS OK"; else echo "CITATION FAILURES PRESENT"; fi
exit "$fail"
