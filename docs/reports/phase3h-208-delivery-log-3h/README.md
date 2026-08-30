# Task 208 — `docs/DELIVERY-LOG.md`'s Phase 3h paragraph, citation checks

`docs/DELIVERY-LOG.md` gained a new **Phase 3h** paragraph (inserted immediately before
`## Other milestones`), naming the PRD path, the run id, requirements R94–R97, the operator
ruling, this phase's final code sha, both gate results quoted from their own evidence
directories, and links to `docs/reports/phase3h.md` / `docs/reports/phase3h-findings.md`. This
report validates every sha and path the new paragraph cites, one quoted check each, tokens
enumerated mechanically from the paragraph itself — not from expectation.

## Locating the paragraph

```
$ grep -n '\*\*Phase 3h\*\*' docs/DELIVERY-LOG.md
712:**Phase 3h** — `prds/phase3h-suite-reconciliation.md`, run deck-phase3h, 2026-08-30. Four
```

The paragraph runs from that line to the following blank line, extracted verbatim with
`awk '/\*\*Phase 3h\*\*/,/^$/' docs/DELIVERY-LOG.md`.

## Every backticked token in the paragraph, enumerated mechanically

```
$ awk '/\*\*Phase 3h\*\*/,/^$/' docs/DELIVERY-LOG.md > /tmp/phase3h_para.txt
$ grep -o '`[^`]*`' /tmp/phase3h_para.txt | sort -u
`4b1d4dc`
`SPEC.md`
`a24ff8d..HEAD`
`a24ff8d`
`ci/Dockerfile`
`ci/SPIKE.md`
`de90a5c..HEAD`
`de90a5c`
`docs/reports/phase3h-202-fullsuite/README.md`
`docs/reports/phase3h-203-fullsuite-verbose/README.md`
`docs/reports/phase3h-204-stability10/README.md`
`docs/reports/phase3h-204-stability10/summary.log`
`docs/reports/phase3h-findings.md`
`docs/reports/phase3h.md`
`features/filter.feature`
`prds/`
`prds/phase3h-suite-reconciliation.md`
```

Seventeen tokens. Every one is classifiable as either a resolvable sha (or a sha range, checked by
both endpoints) or a tracked path — none is a `/tmp` path, a bare basename, a deleted file or a
generated/untracked directory. In particular the out-of-scope stability race the paragraph names
is cited by its full tracked path `features/filter.feature`, with its directory component, not by a
directory-less basename (which `git ls-files --error-unmatch` rejects, since no such path is
tracked at the repository root).

| Token | Kind | Check | Result |
|---|---|---|---|
| `4b1d4dc` | sha | `git cat-file -e 4b1d4dc^{commit}` | exists |
| `a24ff8d` | sha | `git cat-file -e a24ff8d^{commit}` | exists |
| `de90a5c` | sha | `git cat-file -e de90a5c^{commit}` | exists |
| `a24ff8d..HEAD` | range | both endpoints checked (`a24ff8d` above; `HEAD` below) | valid |
| `de90a5c..HEAD` | range | both endpoints checked (`de90a5c` above; `HEAD` below) | valid |
| `SPEC.md` | path | `git ls-files --error-unmatch SPEC.md` | tracked |
| `ci/Dockerfile` | path | `git ls-files --error-unmatch ci/Dockerfile` | tracked |
| `ci/SPIKE.md` | path | `git ls-files --error-unmatch ci/SPIKE.md` | tracked |
| `prds/` | path (dir) | `git ls-files --error-unmatch prds/` | tracked |
| `prds/phase3h-suite-reconciliation.md` | path | `git ls-files --error-unmatch prds/phase3h-suite-reconciliation.md` | tracked |
| `docs/reports/phase3h.md` | path | `git ls-files --error-unmatch docs/reports/phase3h.md` | tracked |
| `docs/reports/phase3h-findings.md` | path | `git ls-files --error-unmatch docs/reports/phase3h-findings.md` | tracked |
| `docs/reports/phase3h-202-fullsuite/README.md` | path | `git ls-files --error-unmatch docs/reports/phase3h-202-fullsuite/README.md` | tracked |
| `docs/reports/phase3h-203-fullsuite-verbose/README.md` | path | `git ls-files --error-unmatch docs/reports/phase3h-203-fullsuite-verbose/README.md` | tracked |
| `docs/reports/phase3h-204-stability10/README.md` | path | `git ls-files --error-unmatch docs/reports/phase3h-204-stability10/README.md` | tracked |
| `docs/reports/phase3h-204-stability10/summary.log` | path | `git ls-files --error-unmatch docs/reports/phase3h-204-stability10/summary.log` | tracked |
| `features/filter.feature` | path | `git ls-files --error-unmatch features/filter.feature` | tracked |

Run as one batch, quoted:

```
$ git cat-file -e 4b1d4dc^{commit} && git cat-file -e a24ff8d^{commit} \
    && git cat-file -e de90a5c^{commit} && git cat-file -e HEAD^{commit} && echo ALL_SHAS_OK
ALL_SHAS_OK
$ git ls-files --error-unmatch SPEC.md ci/Dockerfile ci/SPIKE.md prds/ \
    prds/phase3h-suite-reconciliation.md docs/reports/phase3h.md docs/reports/phase3h-findings.md \
    docs/reports/phase3h-202-fullsuite/README.md docs/reports/phase3h-203-fullsuite-verbose/README.md \
    docs/reports/phase3h-204-stability10/README.md docs/reports/phase3h-204-stability10/summary.log \
    features/filter.feature \
    >/dev/null && echo ALL_PATHS_OK
ALL_PATHS_OK
```

## One quoted check per enumerated token, run individually

The batch above is a convenience; below is the required per-token evidence — every one of the
seventeen tokens enumerated mechanically above gets its own command and its own verbatim output, in
the enumeration's own order. No token is covered by another token's check, and no token is
spot-checked via a sibling path.

```
$ git cat-file -e 4b1d4dc^{commit} && echo SHA_OK
SHA_OK
$ git ls-files --error-unmatch SPEC.md
SPEC.md
$ git cat-file -e a24ff8d^{commit} && git cat-file -e HEAD^{commit} && echo RANGE_ENDPOINTS_OK
RANGE_ENDPOINTS_OK
$ git cat-file -e a24ff8d^{commit} && echo SHA_OK
SHA_OK
$ git ls-files --error-unmatch ci/Dockerfile
ci/Dockerfile
$ git ls-files --error-unmatch ci/SPIKE.md
ci/SPIKE.md
$ git cat-file -e de90a5c^{commit} && git cat-file -e HEAD^{commit} && echo RANGE_ENDPOINTS_OK
RANGE_ENDPOINTS_OK
$ git cat-file -e de90a5c^{commit} && echo SHA_OK
SHA_OK
$ git ls-files --error-unmatch docs/reports/phase3h-202-fullsuite/README.md
docs/reports/phase3h-202-fullsuite/README.md
$ git ls-files --error-unmatch docs/reports/phase3h-203-fullsuite-verbose/README.md
docs/reports/phase3h-203-fullsuite-verbose/README.md
$ git ls-files --error-unmatch docs/reports/phase3h-204-stability10/README.md
docs/reports/phase3h-204-stability10/README.md
$ git ls-files --error-unmatch docs/reports/phase3h-204-stability10/summary.log
docs/reports/phase3h-204-stability10/summary.log
$ git ls-files --error-unmatch docs/reports/phase3h-findings.md
docs/reports/phase3h-findings.md
$ git ls-files --error-unmatch docs/reports/phase3h.md
docs/reports/phase3h.md
$ git ls-files --error-unmatch features/filter.feature
features/filter.feature
$ git ls-files --error-unmatch prds/
prds/phase0-harness-and-skeleton.md
prds/phase0b-harness-hardening.md
prds/phase1-durable-identity-and-agents.md
prds/phase2-status-truth.md
prds/phase2b1-visible-shell.md
prds/phase2b2-configuration-and-appearance.md
prds/phase3-sessions-and-lifecycle.md
prds/phase3b-interactive-preview.md
prds/phase3c-residual-and-interactive-preview.md
prds/phase3e-list-ergonomics-and-chrome.md
prds/phase3f-residuals-and-suite-determinism.md
prds/phase3g-field-backlog.md
prds/phase3h-suite-reconciliation.md
prds/spike-sibling-toolchain.md
prds/spike-tmux-embedded-preview.md
$ git ls-files --error-unmatch prds/phase3h-suite-reconciliation.md
prds/phase3h-suite-reconciliation.md
```

The directory token `prds/` is checked as itself (`git ls-files --error-unmatch prds/`, exit 0,
fifteen tracked files listed) — it is no longer substituted by, or spot-checked through, the
single-file token `prds/phase3h-suite-reconciliation.md`, which carries its own separate check.
Listing every token's command verbatim is deliberate: a reader can re-run the block line by line.

## Path-like tokens the backtick scan alone would miss

Backtick enumeration only finds what is already marked up as code. A second, independent scan
enumerates every path-like token in the paragraph whether or not it is backticked, so that no path
sits in bare prose without a check:

```
$ awk '/\*\*Phase 3h\*\*/,/^$/' docs/DELIVERY-LOG.md \
    | grep -o -E '[A-Za-z0-9_./-]+\.(feature|go|md|log|toml|sh|db|json)' | sort -u
SPEC.md
ci/SPIKE.md
docs/reports/phase3h-202-fullsuite/README.md
docs/reports/phase3h-203-fullsuite-verbose/README.md
docs/reports/phase3h-204-stability10/README.md
docs/reports/phase3h-204-stability10/summary.log
docs/reports/phase3h-findings.md
docs/reports/phase3h.md
features/filter.feature
prds/phase3h-suite-reconciliation.md
reports/phase3h-202-fullsuite/README.md
reports/phase3h-203-fullsuite-verbose/README.md
reports/phase3h-204-stability10/README.md
reports/phase3h-findings.md
reports/phase3h.md
```

Ten of the fifteen are the backticked citations already checked one-by-one above. This scan is what
caught `features/filter.feature`: the paragraph's earlier wording named the out-of-scope stability
race by a directory-less basename, which is not a tracked path, and it now names the tracked
`features/filter.feature` instead. `ci/Dockerfile` and the three shas carry no matching extension
and so appear only in the backtick enumeration, where each has its own check.

The remaining five `reports/…` tokens are not citations: they are the *href halves* of the
paragraph's five Markdown links, written relative to `docs/DELIVERY-LOG.md`'s own directory — the
file's pre-existing convention (`grep -c '](reports/' docs/DELIVERY-LOG.md` → `38`, i.e. every
earlier phase paragraph links the same way), so each resolves under `docs/`. Every token from the
scan, resolved that way where it is a link href, is tracked — and the check is exhaustive rather
than enumerated by hand, so an untracked one could not hide:

```
$ awk '/\*\*Phase 3h\*\*/,/^$/' docs/DELIVERY-LOG.md \
    | grep -o -E '[A-Za-z0-9_./-]+\.(feature|go|md|log|toml|sh|db|json)' | sort -u \
    | while read -r t; do \
        if git ls-files --error-unmatch "$t" >/dev/null 2>&1; then echo "tracked       $t"; \
        elif git ls-files --error-unmatch "docs/$t" >/dev/null 2>&1; then echo "tracked docs/ $t"; \
        else echo "UNTRACKED     $t"; fi; done
tracked       SPEC.md
tracked       ci/SPIKE.md
tracked       docs/reports/phase3h-202-fullsuite/README.md
tracked       docs/reports/phase3h-203-fullsuite-verbose/README.md
tracked       docs/reports/phase3h-204-stability10/README.md
tracked       docs/reports/phase3h-204-stability10/summary.log
tracked       docs/reports/phase3h-findings.md
tracked       docs/reports/phase3h.md
tracked       features/filter.feature
tracked       prds/phase3h-suite-reconciliation.md
tracked docs/ reports/phase3h-202-fullsuite/README.md
tracked docs/ reports/phase3h-203-fullsuite-verbose/README.md
tracked docs/ reports/phase3h-204-stability10/README.md
tracked docs/ reports/phase3h-findings.md
tracked docs/ reports/phase3h.md
```

No `UNTRACKED` line: every path the paragraph cites in prose, in backticks or as a link target
exists in the index.

## The two range claims the paragraph makes, re-verified

```
$ git log --oneline de90a5c..HEAD -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
a24ff8d plan: add Phase 3h — bring the suite to the §7 ruling, fix F31, hold the gate at one sha (operator)
$ git log --oneline a24ff8d..HEAD -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
(nothing printed)
$ git diff --stat a24ff8d..HEAD -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
(nothing printed)
```

Matches the paragraph's claim exactly: `de90a5c..HEAD` over the protected paths contains exactly
one commit, `a24ff8d` (the operator's own pre-launch commit adding this phase's PRD), and
`a24ff8d..HEAD` over the same paths is empty both by commit log and by diff.

## The three quoted facts the paragraph states, re-verified against their own evidence

Whole-suite sweep exit status (task 202):

```
$ cat docs/reports/phase3h-202-fullsuite/.exitstatus
0
```

Verbose companion's Gherkin tally (task 203), quoted with line numbers:

```
$ grep -a -n -E '^[0-9]+ scenarios \(|^[0-9]+ steps \(' docs/reports/phase3h-203-fullsuite-verbose/verbose.log | head -2
5445:311 scenarios ([32m311 passed[0m)
5446:3532 steps ([32m3532 passed[0m)
```

Stability gate's summary final line (task 204), quoted verbatim:

```
$ tail -1 docs/reports/phase3h-204-stability10/summary.log
10/10 passed
```

## Final code sha invariance (this task added no code)

```
$ git diff --stat 4b1d4dc..HEAD -- '*.go' '*.feature'
(nothing printed)
$ git log -1 --format=%H -- '*.go' '*.feature'
4b1d4dcbd4480013470a0555795e6c64db3bf96d
```

Still task 201's `4b1d4dc`, unchanged — this task is a docs-only descendant.

## Protected paths

```
$ git log --oneline a24ff8d..HEAD -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
(nothing printed)
$ git diff --stat a24ff8d..HEAD -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
(nothing printed)
```

## Commit accounting for this task's marker

Exactly one commit in `a24ff8d..HEAD` carries this task's marker in its subject:

```
$ git log --format='%s' a24ff8d..HEAD | grep -c -F '(task 208)' && git log --format='%h %s' a24ff8d..HEAD | grep -F '(task 208)'
1
87bde8f docs: add Phase 3h's DELIVERY-LOG paragraph at the final code sha with both gate results (task 208)
```

The two later commits touching this directory (`0d9a551`, `f54873e`) are unmarked docs-only
refinements of the citation checks in this very report; the paragraph never names its own sha, so no
marker-carrying addendum was permitted and none was made. `f54873e`'s message body mentions the
marker string only inside a sentence explaining that the commit deliberately does not carry it —
subject lines, which is where the marker convention lives, count one.

## Outcome

- `docs/DELIVERY-LOG.md` gained one new paragraph, introduced by `**Phase 3h**`, immediately
  before `## Other milestones`.
- Every sha and path token the paragraph cites resolves/is tracked, checked above — by backtick
  enumeration (seventeen tokens, one command each) and by an independent path-like scan that also
  covers bare prose and the Markdown links' href halves.
- No protected path touched. No code touched (final code sha unchanged at `4b1d4dc`).
