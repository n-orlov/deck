"""Phase 3g close-out citation sweep (task 113).

Scans exactly the two phase-3g aggregator reports named by task 113's success
criteria -- docs/reports/phase3g.md and docs/reports/phase3g-findings.md --
and proves, for each:

  1. every backtick-quoted sha it cites resolves to a real commit
     (`git cat-file -e <sha>^{commit}`), skipping purely-decimal tokens (unix
     timestamps in the stability tables, not shas);
  2. every relative markdown link target it cites exists, resolved against
     docs/reports/ (both files live directly there, so this is the correct
     base -- unlike a per-task report living in a subdirectory);
  3. every same-document '](#...)' anchor resolves against that file's own
     headings, using GitHub's slug rule (strip backticks, drop punctuation,
     spaces to hyphens -- a run of punctuation collapses to a DOUBLE hyphen
     because the punctuation itself is dropped while both surrounding spaces
     still become hyphens, the em-dash/ellipsis gotcha task 110 hit).

Known false-positive class (named in prds/phase3g-field-backlog.md's "Reports"
section, and already disclosed once in docs/reports/phase3g-findings.md's own
S4): a sweep that greps a report's own prose for a citation/keyword will flag
the report describing the thing as if it were a fresh instance of it. This
sweep does not do that -- it checks *resolution* (does the sha/path/anchor
exist), never keyword presence -- so that specific class cannot fire here.
The place it legitimately fires in this tree is disclosed, not re-triggered:
phase3g-findings.md S4's F2-recurrence grep explicitly excludes its own file
before running, for exactly this reason (the section's own prose contains the
words "not settled"/"settle" while describing the flake it is checking for,
which would otherwise self-match). See README.md S2 for the citation.

Run from the repo root: python3 docs/reports/phase3g-113-closeout/citation_sweep.py
Exit 0 iff there are zero failures.
"""

import os
import re
import subprocess
import sys

root = os.getcwd()
files = ["docs/reports/phase3g.md", "docs/reports/phase3g-findings.md"]

sha_re = re.compile(r"`([0-9a-f]{7,40})`")
link_re = re.compile(r"\[[^\]]*\]\(([^)]+)\)")
heading_re = re.compile(r"(?m)^#+ (.*)$")


def slug(t):
    t = t.replace("`", "")
    t = re.sub(r"[^\w\- ]", "", t.lower(), flags=re.UNICODE)
    return t.replace(" ", "-")


shas = {}       # sha -> [citing file, ...]
links = {}      # link target (relative to docs/reports/) -> [citing file, ...]
anchors = {}    # (file, anchor) -> True  (checked against that file's own headings)

for f in files:
    text = open(os.path.join(root, f), encoding="utf-8").read()
    headings = {slug(m.group(1)) for m in heading_re.finditer(text)}
    for m in sha_re.finditer(text):
        s = m.group(1)
        if s.isdigit():
            continue
        shas.setdefault(s, []).append(f)
    for m in link_re.finditer(text):
        target = m.group(1)
        if target.startswith("#"):
            anchors[(f, target[1:])] = target[1:] in headings
            continue
        target = target.split("#")[0]
        if not target or target.startswith(("http://", "https://", "mailto:")):
            continue
        links.setdefault(target, []).append(f)

sha_failures = [s for s in sorted(shas)
                if subprocess.run(["git", "cat-file", "-e", s + "^{commit}"],
                                   cwd=root, capture_output=True).returncode != 0]

link_failures = [t for t in sorted(links)
                  if not os.path.exists(os.path.join(root, "docs/reports", t))]

anchor_failures = [(f, a) for (f, a), ok in sorted(anchors.items()) if not ok]

print(f"reports scanned: {len(files)}")
for f in files:
    print(f"  {f}")
print()
print(f"distinct shas cited: {len(shas)}")
print(f"sha resolution failures: {len(sha_failures)}")
for s in sha_failures:
    print(f"  FAIL sha {s} (cited by {', '.join(sorted(set(shas[s])))})")
print()
print(f"distinct link targets cited: {len(links)}")
print(f"link resolution failures: {len(link_failures)}")
for t in link_failures:
    print(f"  FAIL link {t} (cited by {', '.join(sorted(set(links[t])))})")
print()
print(f"distinct same-document anchors cited: {len(anchors)}")
print(f"anchor resolution failures: {len(anchor_failures)}")
for f, a in anchor_failures:
    print(f"  FAIL anchor #{a} in {f}")
print()
if sha_failures or link_failures or anchor_failures:
    print("CITATION SWEEP: FAIL")
    sys.exit(1)
print("CITATION SWEEP: PASS - every cited sha resolves, every cited path exists, "
      "every same-document anchor resolves")
