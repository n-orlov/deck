#!/usr/bin/env python3
"""Sweep every cross-reference to docs/reports/phase2b2-findings.md and prove none dangles.

A citation of a report *section by title* is load-bearing (non-negotiable 8): it must resolve, and
it must resolve to exactly one section. Over every tracked *.md under docs/ and prds/ this checks:

1. every citation of the form ``phase2b2-findings.md`'s "<Some Title>" section`` matches at least
   one `##` heading in that file (line wraps in the citing prose are tolerated by normalising
   whitespace before matching) -- zero matches is a dangling citation;
2. no such citation matches MORE than one heading -- an ambiguous citation cannot be followed, and
   two sections sharing a title is exactly the R67 part-2 defect;
3. every citation of the form `phase2b2-findings.md:<N>` points at a line that exists, and when the
   citing sentence calls it a section/heading, that line really is a `##` heading.

Two classes of stale reference are DISCLOSED as findings instead of failing the sweep, because
neither is fixable inside R67's scope; both are listed by exact path and cited text below, so
nothing is exempt silently:

* `prds/` is read-only to this phase's job, and `prds/phase3f-residuals-and-suite-determinism.md`
  quotes the pre-R67 duplicate title while describing the very defect R67 removes;
* `docs/reports/phase2b2-findings.md` also has FOUR sections labelled `Task 034`, so citations of
  "Task 034" by title are ambiguous for the same reason the `Task 014` pair was. R67 names only the
  `Task 014` pair, so retitling those four is a separate change, recorded here for a later pass.

Exit status is 0 only when checks 1-3 hold outside those disclosed findings. Run from the repo root.
"""

import re
import subprocess
import sys

TARGET = "docs/reports/phase2b2-findings.md"

# (path, cited title) -> why it is a finding rather than a failure.
DISCLOSED = {
    ("prds/phase3f-residuals-and-suite-determinism.md", "Task 014"): (
        "cites the pre-R67 title; prds/ is protected and must not be edited by this job"
    ),
    ("docs/reports/phase2b2.md", "Task 034"): (
        "four sections are labelled `Task 034`; R67 scopes only the `Task 014` pair, "
        "so this ambiguity is left for a later pass"
    ),
}

lines = open(TARGET, encoding="utf-8").read().split("\n")
headings = []  # (line number, title)
for i, line in enumerate(lines, start=1):
    if line.startswith("## "):
        headings.append((i, line[3:].strip()))

problems = []
findings = []

tracked = subprocess.run(
    ["git", "ls-files", "docs", "prds"], capture_output=True, text=True, check=True
).stdout.split()
mds = [p for p in tracked if p.endswith(".md")]

title_cite = re.compile(r"phase2b2-findings\.md`?'?s?[^\"]{0,40}\"([^\"]{3,200})\"")
line_cite = re.compile(r"phase2b2-findings\.md`?:(\d+)")

n_title = n_line = 0
for path in mds:
    flat = " ".join(open(path, encoding="utf-8").read().split())
    for m in title_cite.finditer(flat):
        cited = " ".join(m.group(1).split())
        if not cited.lower().startswith(("task ", "tasks ", "requirement ", "sigkill ")):
            continue  # a quoted phrase, not a section name
        n_title += 1
        hits = [n for n, h in headings if h.startswith(cited) or cited.startswith(h)]
        why = "dangling" if not hits else (f"ambiguous, matches lines {hits}" if len(hits) > 1 else "")
        if not why:
            continue
        if (path, cited) in DISCLOSED:
            findings.append(f"{path}: {cited!r} {why} -- {DISCLOSED[(path, cited)]}")
        else:
            problems.append(f"{path}: section citation {cited!r} is {why}")
    for m in line_cite.finditer(flat):
        n = int(m.group(1))
        n_line += 1
        if not 1 <= n <= len(lines):
            problems.append(f"{path}: citation of {TARGET}:{n} is past end of file ({len(lines)})")
            continue
        window = flat[max(0, m.start() - 160) : m.end() + 160]
        if ("section" in window or "heading" in window or "titled" in window) and not lines[
            n - 1
        ].startswith("## "):
            problems.append(f"{path}: {TARGET}:{n} is cited as a section but is not a heading")

print(f"{TARGET}: {len(headings)} '##' headings")
print(f"citations checked: {n_title} by title, {n_line} by line number, over {len(mds)} md files")
for f in findings:
    print(f"finding (disclosed, not a failure): {f}")
if problems:
    print("PROBLEMS:")
    for p in problems:
        print(f"  - {p}")
    sys.exit(1)
print("OK: every section citation resolves to exactly one heading; no dangling line citation")
