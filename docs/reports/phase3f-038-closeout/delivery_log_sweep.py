#!/usr/bin/env python3
"""Manual citation sweep for docs/DELIVERY-LOG.md.

Neither committed checker looks at docs/DELIVERY-LOG.md:
docs/reports/phase3f-evidence/check-citations.sh reads only the two aggregators
(docs/reports/phase3f.md, docs/reports/phase3f-findings.md), and
docs/reports/phase3f-027-closeout/citation_sweep.py walks only *.md under
docs/reports/ whose path contains "phase3f". So the delivery log's citations are
swept here, by hand, from the repo root.

Unlike the two committed checkers this one does NOT demand that every hex token
resolve to a commit, because the delivery log's "PRD" column deliberately cites
the *blob* sha of the PRD file version a run was given, and two rows cite shas in
a different repository (n-orlov/ralphd). Tokens are therefore classified:

  commit  - resolves in this repo as a commit          (accepted)
  blob    - resolves in this repo as a blob            (accepted: PRD file sha)
  foreign - does not resolve here, and the citing line says why: either a
            named other repo or an "in-run snapshot" PRD version that was never
            committed here                             (accepted, listed)
  UNEXPLAINED - anything else                          (hard failure)
"""
import os
import re
import subprocess
import sys

DOC = "docs/DELIVERY-LOG.md"
FOREIGN_MARKERS = ("n-orlov/ralphd", "in-run snapshot", "ralphd")


def object_type(sha):
    r = subprocess.run(["git", "cat-file", "-t", sha], capture_output=True, text=True)
    return r.stdout.strip() if r.returncode == 0 else None


def main():
    text = open(DOC).read()
    lines = text.splitlines()
    tokens = sorted({t for t in re.findall(r"`([0-9a-f]{7,40})`", text) if not t.isdigit()})

    kinds = {"commit": [], "blob": [], "foreign": [], "UNEXPLAINED": []}
    for tok in tokens:
        kind = object_type(tok)
        if kind in ("commit", "blob"):
            kinds[kind].append(tok)
            continue
        citing = [ln for ln in lines if tok in ln]
        if any(m in ln for ln in citing for m in FOREIGN_MARKERS):
            kinds["foreign"].append(tok)
        else:
            kinds["UNEXPLAINED"].append(tok)

    links = sorted(
        l for l in set(re.findall(r"\]\(([^)#][^)]*)\)", text))
        if not l.startswith(("http://", "https://", "mailto:"))
    )
    bad_links = [l for l in links if not os.path.exists(os.path.normpath(os.path.join(os.path.dirname(DOC), l)))]

    paths = sorted(set(re.findall(r"`((?:docs|internal|features|cmd|ci|prds)/[A-Za-z0-9_./-]+)`", text)))
    bad_paths = [p for p in paths if not os.path.exists(p)]

    print("document:", DOC)
    print("distinct hex tokens cited:", len(tokens))
    for kind in ("commit", "blob", "foreign", "UNEXPLAINED"):
        print("  %-11s %2d  %s" % (kind, len(kinds[kind]), " ".join(kinds[kind])))
    print("distinct relative link targets:", len(links), "- failures:", len(bad_links), bad_links or "")
    print("distinct backticked repo paths:", len(paths), "- failures:", len(bad_paths), bad_paths or "")

    failed = kinds["UNEXPLAINED"] or bad_links or bad_paths
    print("VERDICT:", "FAIL" if failed else "PASS - no unexplained sha, every link and path resolves")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
