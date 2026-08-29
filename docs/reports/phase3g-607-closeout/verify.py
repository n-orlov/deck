#!/usr/bin/env python3
"""Re-derivable checker for the close-out section of docs/reports/phase3g.md (task 607).

Run from the repository root:

    python3 docs/reports/phase3g-607-closeout/verify.py

Checks, in order:
  1. the section `## Close-out (approach 06)` exists and its GitHub-style anchor slug
     matches the link in the file's own section list;
  2. every sha the section quotes in backticks resolves under `git cat-file -e <sha>^{commit}`;
  3. every markdown link target in the section resolves on disk (relative to docs/reports/),
     skipping in-document `#anchor` links, which are checked against the file's own headings;
  4. every row of the (f) exceptional-status table names at least one sha AND at least one
     tracked evidence path (a link), or points at a findings row (F-number) for the residual;
  5. the code-pattern diff back to the final code sha b0a4e7d is empty.

Exit status 0 means every check passed; 1 means at least one failed. Nothing is written.
"""
import os
import re
import subprocess
import sys

ROOT = subprocess.run(["git", "rev-parse", "--show-toplevel"], capture_output=True,
                      text=True, check=True).stdout.strip()
REPORT = os.path.join(ROOT, "docs", "reports", "phase3g.md")
BASE = os.path.dirname(REPORT)
HEADING = "## Close-out (approach 06)"
ANCHOR = "close-out-approach-06"
FINAL_CODE_SHA = "b0a4e7d"
CODE_PATTERNS = ["*.go", "*.feature", "*.sh", "*.toml", "go.mod", "go.sum"]

failures = []
text = open(REPORT, encoding="utf-8").read()
lines = text.splitlines()


def slug(title):
    s = title.strip().lower()
    s = re.sub(r"[^\w\s-]", "", s)
    return re.sub(r"\s+", "-", s)


# ---- 1. section exists, anchor resolves, section list links to it ------------------
start = next((i for i, l in enumerate(lines) if l.strip() == HEADING), None)
if start is None:
    print("FAIL 1: section heading %r not found" % HEADING)
    sys.exit(1)
end = next((i for i in range(start + 1, len(lines))
            if lines[i].startswith("## ")), len(lines))
section = lines[start:end]
print("OK   1a: %r at line %d, %d lines" % (HEADING, start + 1, len(section)))

computed = slug(HEADING[3:])
if computed != ANCHOR:
    failures.append("1b: heading slug %r != anchor %r" % (computed, ANCHOR))
else:
    print("OK   1b: heading slug == anchor #%s" % ANCHOR)

listed = [i + 1 for i, l in enumerate(lines[:start])
          if "(#%s)" % ANCHOR in l]
if not listed:
    failures.append("1c: no link to #%s above the section (section list)" % ANCHOR)
else:
    print("OK   1c: linked from the file's own section list at line(s) %s" %
          ", ".join(map(str, listed)))

# ---- 2. every quoted sha resolves ------------------------------------------------
body = "\n".join(section)
shas = sorted(set(m for m in re.findall(r"`([0-9a-f]{7,40})`", body)))
bad = []
for sha in shas:
    r = subprocess.run(["git", "-C", ROOT, "cat-file", "-e", sha + "^{commit}"],
                       capture_output=True)
    if r.returncode != 0:
        bad.append(sha)
if bad:
    failures.append("2: shas that do not resolve: %s" % ", ".join(bad))
print("%s   2: %d quoted shas checked, %d unresolved" %
      ("OK " if not bad else "FAIL", len(shas), len(bad)))

# ---- 3. every link target resolves ------------------------------------------------
links = re.findall(r"\]\(([^)]+)\)", body)
headings = {slug(l.lstrip("#").strip()) for l in lines if l.startswith("#")}
missing = []
for target in links:
    if target.startswith("#"):
        if target[1:] not in headings:
            missing.append(target + " (in-document anchor)")
        continue
    path, _, frag = target.partition("#")
    if not os.path.exists(os.path.join(BASE, path)):
        missing.append(target)
if missing:
    failures.append("3: link targets that do not resolve: %s" % ", ".join(missing))
print("%s   3: %d link targets checked, %d unresolved" %
      ("OK " if not missing else "FAIL", len(links), len(missing)))

# ---- 4. every (f) row names a sha and an evidence path (or a findings residual) ----
frows = [l for l in section
         if re.match(r"\|\s*(0\d\d|1\d\d|2\d\d|3\d\d|5\d\d)\s*\|", l)]
weak = []
for row in frows:
    cells = [c.strip() for c in row.strip().strip("|").split("|")]
    task, delivered = cells[0], cells[-1]
    has_sha = bool(re.search(r"`[0-9a-f]{7,40}`", delivered))
    has_path = bool(re.search(r"\]\([^)#][^)]*\)", delivered))
    has_finding = bool(re.search(r"\bF\d\d\b", delivered))
    if not (has_sha and (has_path or has_finding)):
        weak.append("%s (sha=%s path=%s finding=%s)" %
                    (task, has_sha, has_path, has_finding))
if weak:
    failures.append("4: rows without both a sha and an evidence path/findings row: %s"
                    % "; ".join(weak))
print("%s   4: %d (f) rows checked, %d weak" %
      ("OK " if not weak else "FAIL", len(frows), len(weak)))

# ---- 5. the code-pattern diff back to the final code sha is empty ------------------
r = subprocess.run(["git", "-C", ROOT, "diff", "--stat",
                    "%s..HEAD" % FINAL_CODE_SHA, "--"] + CODE_PATTERNS,
                   capture_output=True, text=True)
if r.returncode != 0 or r.stdout.strip():
    failures.append("5: code diff back to %s is not empty:\n%s" %
                    (FINAL_CODE_SHA, r.stdout))
else:
    print("OK   5: git diff --stat %s..HEAD over %s is empty (exit %d)" %
          (FINAL_CODE_SHA, " ".join(CODE_PATTERNS), r.returncode))

print()
if failures:
    for f in failures:
        print("FAIL " + f)
    sys.exit(1)
print("ALL CHECKS PASSED")
