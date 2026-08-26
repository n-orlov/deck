"""Phase 3f close-out citation sweep (task 027).

Scans EVERY phase-3f report markdown file under docs/reports/ and proves:
  1. every backticked sha it cites resolves to a real commit (`git cat-file -e <s>^{commit}`);
  2. every relative markdown link target it cites exists, resolved against THE CITING
     REPORT'S OWN DIRECTORY -- not against docs/reports/.

(2) is why this exists alongside docs/reports/phase3f-evidence/check-citations.sh: that
script resolves every link as docs/reports/<target>, which is right for reports sitting
directly in docs/reports/ but wrong for reports inside a subdirectory (a link to
`./go-test-p1-count1-all.log` from phase3f-021-fullsuite/README.md, or to `../phase3f.md`
from phase3f-evidence/README.md, is reported missing although the file is there). This
sweep resolves per citing directory, and demands that a target resolve from EVERY
directory that cites it -- not merely from one of them.

Run from the repo root: python3 docs/reports/phase3f-027-closeout/citation_sweep.py
Exit 0 iff there are zero failures.
"""

import os
import re
import subprocess
import sys

root = os.getcwd()
files = sorted(subprocess.run(
    ["find", "docs/reports", "-path", "*phase3f*", "-name", "*.md"],
    cwd=root, capture_output=True, text=True, check=True).stdout.split())

sha_re = re.compile(r"`([0-9a-f]{7,40})`")
link_re = re.compile(r"\[[^\]]*\]\(([^)]+)\)")

shas = {}   # sha -> [citing report, ...]
links = {}  # (citing dir, target) -> [citing report, ...]

for f in files:
    text = open(os.path.join(root, f), encoding="utf-8").read()
    for m in sha_re.finditer(text):
        s = m.group(1)
        if s.isdigit():
            continue  # a ten-digit unix timestamp is not a sha
        shas.setdefault(s, []).append(f)
    for m in link_re.finditer(text):
        target = m.group(1).split("#")[0]
        if not target or target.startswith(("http://", "https://", "mailto:")):
            continue
        links.setdefault((os.path.dirname(f), target), []).append(f)

sha_failures = [s for s in sorted(shas)
                if subprocess.run(["git", "cat-file", "-e", s + "^{commit}"],
                                  cwd=root, capture_output=True).returncode != 0]

link_failures = [(d, t) for (d, t) in sorted(links)
                 if not os.path.exists(os.path.normpath(os.path.join(root, d, t)))]

print(f"reports scanned: {len(files)}")
for f in files:
    print(f"  {f}")
print()
print(f"distinct shas cited: {len(shas)}")
print(f"sha resolution failures: {len(sha_failures)}")
for s in sha_failures:
    print(f"  FAIL sha {s} (cited by {', '.join(sorted(set(shas[s])))})")
print()
print(f"distinct (citing dir, link target) pairs: {len(links)}")
print(f"link resolution failures: {len(link_failures)}")
for d, t in link_failures:
    print(f"  FAIL link {t} from {d or '.'} "
          f"(cited by {', '.join(sorted(set(links[(d, t)])))})")
print()
if sha_failures or link_failures:
    print("CITATION SWEEP: FAIL")
    sys.exit(1)
print("CITATION SWEEP: PASS - every cited sha resolves and every cited path exists")
