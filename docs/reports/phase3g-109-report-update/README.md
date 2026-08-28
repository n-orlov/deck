# Task 109 — citation sweep over `docs/reports/phase3g.md`

Two mechanical sweeps over the report as of `fe50efe` (the commit that folded tasks
101–108 into it). Both are re-derivable by the commands quoted below; the captured
output is checked in beside this README.

## 1. Every cited sha resolves — `sha-resolution.log`

Extract every backtick-quoted hex run of 7–40 chars from `docs/reports/phase3g.md`,
de-duplicate, and `git cat-file -e <sha>^{commit}` each one:

```
python3 - <<'PY'
import re, subprocess
txt = open('docs/reports/phase3g.md').read()
shas = sorted(set(re.findall(r'`([0-9a-f]{7,40})`', txt)))
print("extracted %d distinct backtick shas" % len(shas))
bad=[]
for s in shas:
    r = subprocess.run(['git','cat-file','-e',s+'^{commit}'],capture_output=True,text=True)
    print("%s %s" % ("OK  " if r.returncode==0 else "FAIL", s), r.stderr.strip())
    if r.returncode: bad.append(s)
print("unresolvable:", bad if bad else "none")
PY
```

Result: **64 distinct shas, 64 resolve, `unresolvable: none`.** That set includes all
ten shas this approach landed (`2549406`, `89682e5`, `51b7f17`, `045a6e6`, `ea6ce4b`,
`57a6882`, `0219e42`, `02e64a5`, `ab34cb4`, `860c412`) plus the run base `1cfbd5a` and
the pre-existing operator commits `2eed8de`/`2b39124`.

## 2. Every local evidence link resolves — `link-resolution.log`

Run from `docs/reports/`. Two passes: every markdown link target (`](...)`, http(s)
skipped, anchor stripped), then every backtick-quoted `docs/reports/phase3g-*` path
resolved against the repo root.

```
cd docs/reports && python3 - <<'PY'
import re, os
txt = open('phase3g.md').read()
for l in dict.fromkeys(re.findall(r'\]\(([^)]+)\)', txt)):
    if l.startswith(('http://','https://')): continue
    p = l.split('#')[0]
    if p: print(("OK   " if os.path.exists(p) else "FAIL ")+l)
for p in sorted(set(re.findall(r'`(docs/reports/phase3g-[^`]*?)`', txt))):
    print(("OK   " if os.path.exists(os.path.join('/workspace',p)) else "FAIL ")+p)
PY
```

Result: **29 distinct markdown links, all resolve.** Of the 33 backtick-quoted evidence
paths, 32 resolve; the one non-resolving string is **not a link and not an evidence
claim** — it is the glob `docs/reports/phase3g-02[56]-*/` inside R86's disclosure that
tasks 025/026 produced *no* evidence directory at implementation time. Its non-existence
is the disclosure's point; making it resolve would falsify the report.

## What was *not* touched

R86's and R91's disclosures that their red/green pairs were captured **retroactively by
task 038**, not at implementation time, are unchanged by `fe50efe` — verifiable with
`git diff 860c412 fe50efe -- docs/reports/phase3g.md`, whose deletions are confined to
the stale "task 022 is pending / not yet done" paragraphs and the superseded
per-requirement table rows.
