import re, subprocess, os, sys

root = "/workspace"
files = subprocess.run(
    ["find", "docs/reports", "-path", "*phase3e*", "-name", "*.md"],
    cwd=root, capture_output=True, text=True, check=True
).stdout.split()
files.sort()

sha_re = re.compile(r'`([0-9a-f]{7,40})`')
link_re = re.compile(r'\[[^\]]*\]\(([^)]+)\)')

all_shas = set()
all_links = {}  # link target -> resolving base dir(s) where cited

for f in files:
    text = open(os.path.join(root, f), encoding="utf-8").read()
    for m in sha_re.finditer(text):
        s = m.group(1)
        # filter out things that are clearly not hex-plausible shas but could be false positive
        if not s.isdigit():
            all_shas.add(s)
    for m in link_re.finditer(text):
        target = m.group(1)
        if target.startswith("http://") or target.startswith("https://"):
            continue
        if target.startswith("#"):
            continue
        # strip any anchor fragment
        target = target.split("#")[0]
        if not target:
            continue
        basedir = os.path.dirname(os.path.join(root, f))
        all_links.setdefault(target, set()).add(basedir)

sha_failures = []
for s in sorted(all_shas):
    r = subprocess.run(["git", "cat-file", "-e", s], cwd=root)
    if r.returncode != 0:
        sha_failures.append(s)

link_failures = []
for target, basedirs in sorted(all_links.items()):
    resolved_any = False
    for basedir in basedirs:
        p = os.path.normpath(os.path.join(basedir, target))
        if os.path.exists(p):
            resolved_any = True
            break
    if not resolved_any:
        link_failures.append((target, sorted(basedirs)))

print(f"Files scanned: {len(files)}")
for f in files:
    print(f"  {f}")
print()
print(f"Total distinct shas cited: {len(all_shas)}")
print(f"Sha resolution failures: {len(sha_failures)}")
for s in sha_failures:
    print(f"  FAIL sha: {s}")
print()
print(f"Total distinct link targets cited (excluding external): {len(all_links)}")
print(f"Link resolution failures: {len(link_failures)}")
for t, bd in link_failures:
    print(f"  FAIL link: {t} (cited from {bd})")

if sha_failures or link_failures:
    sys.exit(1)
