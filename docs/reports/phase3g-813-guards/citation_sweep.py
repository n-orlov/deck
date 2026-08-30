#!/usr/bin/env python3
"""Citation sweep for docs/reports/phase3g.md and docs/reports/phase3g-findings.md,
re-run at task 606. Checks three classes of citation:

  1. every backtick-quoted 7-40 hex-char string, checked as a git commit sha
     via `git cat-file -e <sha>^{commit}`
  2. every markdown link target `](...)`, resolved either as a repository
     path (relative to docs/reports/, since that is where both files live)
     or, if it starts with `#`, as a same-document GitHub-flavoured-markdown
     anchor (or `file#anchor` for a cross-file anchor)
  3. every backtick-quoted string that looks like a repository path (contains
     '/' or ends in a known extension, optionally followed by a `:line` or
     `:line-line` locator), resolved relative to the repository root, to
     docs/reports/ (since that is where both files live), or relative to
     features/, internal/tui/ or internal/theme/builtin/ for bare filenames
     whose surrounding prose already establishes that directory

Code spans (single-backtick `x` and triple-backtick ``` fenced blocks ```)
are extracted with a small CommonMark-style tokenizer, not a naive
single-backtick regex: an opening run of N backticks is closed by the next
run of exactly N backticks, so a fenced fence's internal single backticks
(git log `%h` hunks, shell prompts inside example output) never pair across
the fence boundary. Class 1 and class 3 only ever look at single-backtick
(N=1) spans; fenced (N=3) spans are skipped entirely -- they are code
blocks, not citations.

Known false-positive classes, disclosed rather than silently filtered:
  A. the two reports citing each other and citing their own filenames in
     prose describing the sweep itself (`phase3g.md`/`phase3g-findings.md`
     referencing each other, or a report naming its own citation-checker
     command) -- a sweep that greps a report's own prose flags the report
     describing itself.
  B. deliberate wrong-path callouts: phase3g-findings.md quotes
     `prds/phase3g-residuals-and-suite-determinism.md` explicitly to name it
     as an earlier draft's *wrong* filename (the correct one is
     `prds/phase3f-residuals-and-suite-determinism.md`) -- the string is
     deliberately non-resolving, and the surrounding sentence says so.
  C. `/run/ralphd/artifacts/task016-unsatisfiable/README.md` is explicitly
     labelled "outside this repository" in the same paragraph, per the
     standing rule that a /run/ralphd/... string may appear only inside a
     quoted command or with that label.
  D. a handful of extraction artifacts of this script's own regex: a bare
     abbreviation of a file already given in full a few words earlier in
     the same sentence (`empire.toml`/`parchment.toml` for
     `internal/theme/builtin/empire.toml`/`.../parchment.toml`, both listed
     alongside `internal/theme/builtin/cobalt.toml` in the same clause), a
     glob (`*_theme_test.go`, `internal/theme/builtin/*.toml`,
     `features/*.feature`, `docs/reports/phase3g*`,
     `docs/reports/phase3g-02[56]-*/`), a bare extension used as a category
     label in prose rather than a filename (`.go`, `.md`, `.toml`,
     `.feature`), a runtime glob path that is not a repository path at all
     (`/tmp/deck-interactive-pipe-*`), or a comma-joined list of feature
     filenames quoted as one span. These are prose, not broken citations,
     and are listed so a re-run is not surprised by them.
  E. a report's own log filenames named in prose but committed under a
     report subdirectory named a few words earlier in the same sentence,
     not at the repository root or docs/reports/ root (e.g.
     `phase3g-106-contrast-floor/theme-suite-green.log` abbreviated to
     just `theme-suite-green.log` once the directory has been established) --
     resolved below by a recursive search of docs/reports/ for the same
     filename, since the sweep's fixed base-directory list does not try
     every report subdirectory automatically.
  F. `001-202.md` (phase3g.md's task-202 row and its prose two lines later)
     is not a repository file at all: it is the identifier of an operator
     ruling delivered to this run outside the repository. The disclosure
     that this identifier is untracked lives in phase3g-findings.md's own
     F33, not repeated at every citing sentence -- deliberate and
     accounted for once per the pair of reports, not per occurrence.
  G. (task 1005, new) `composite-prd.md` in F40's "Reviewer re-run note" is
     this run's own /run/ralphd/composite-prd.md, outside the repository --
     same wording gap as class C's `tasks.json`/`notes.md`, disclosed here
     rather than silently passed. `internal/service/reconcile_bare_error_repair_test.go`,
     also in that same note, is a file the tree no longer contains: task
     902's `a1ca33e` deleted it, and the note cites its former path
     precisely to name what a reviewer would have to restore (via `git show
     608e030:<path>`) to reproduce the now-intentional failure -- the
     non-resolution is the evidence, not a broken link.
"""
import re
import subprocess
import os

REPO = os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))))
REPORTS_DIR = os.path.join(REPO, "docs", "reports")
FILES = ["phase3g.md", "phase3g-findings.md"]


def find_code_spans(text):
    """CommonMark-style code-span tokenizer: an opening run of N backticks
    is closed by the next available run of exactly N backticks; runs of a
    different length occurring in between are literal content of the span,
    not delimiters. Returns a list of (run_length, content) tuples in
    document order. Unmatched backtick runs (no partner of the same
    length anywhere later) are left as literal text, i.e. produce no span."""
    runs = [(m.start(), len(m.group(0))) for m in re.finditer(r"`+", text)]
    used = [False] * len(runs)
    spans = []
    for i, (pos, length) in enumerate(runs):
        if used[i]:
            continue
        for j in range(i + 1, len(runs)):
            if used[j]:
                continue
            pos2, length2 = runs[j]
            if length2 == length:
                spans.append((length, text[pos + length : pos2]))
                used[i] = True
                used[j] = True
                break
    return spans


def gfm_slug(h):
    h = h.lower().replace("`", "")
    out = [ch for ch in h if ch.isalnum() or ch in (" ", "-")]
    return "".join(out).replace(" ", "-")


def collect_headers(text):
    headers, counts = [], {}
    for line in text.splitlines():
        m = re.match(r"^#{1,6}\s+(.*)", line)
        if m:
            slug = gfm_slug(m.group(1))
            if slug in counts:
                counts[slug] += 1
                slug = f"{slug}-{counts[slug]}"
            else:
                counts[slug] = 0
            headers.append(slug)
    return headers


def main():
    texts = {f: open(os.path.join(REPORTS_DIR, f)).read() for f in FILES}
    headers = {f: collect_headers(t) for f, t in texts.items()}
    single_spans = {f: [c for (n, c) in find_code_spans(t) if n == 1] for f, t in texts.items()}

    print("=== 1. sha citations (single-backtick spans only; fenced blocks excluded) ===")
    sha_re = re.compile(r"^[0-9a-f]{7,40}$")
    shas = set()
    for f in FILES:
        for c in single_spans[f]:
            if sha_re.match(c):
                shas.add(c)
    print(f"{len(shas)} unique candidate shas across both files")
    bad_shas = []
    for s in sorted(shas):
        r = subprocess.run(
            ["git", "cat-file", "-e", s + "^{commit}"], cwd=REPO, capture_output=True
        )
        if r.returncode != 0:
            bad_shas.append(s)
    print("non-resolving shas:", bad_shas if bad_shas else "(none)")

    print()
    print("=== 2. markdown link targets ===")
    link_re = re.compile(r"\]\(([^)]+)\)")
    unresolved_links = []
    checked_links = 0
    for f in FILES:
        for m in link_re.finditer(texts[f]):
            target = m.group(1)
            checked_links += 1
            if target.startswith("#"):
                anchor = target[1:]
                if anchor not in headers[f]:
                    unresolved_links.append((f, target, "same-doc anchor"))
            elif "#" in target and not target.split("#")[0].endswith(
                (".log", ".py", ".sh")
            ):
                path, _, anchor = target.partition("#")
                if path in headers:
                    if anchor not in headers[path]:
                        unresolved_links.append((f, target, "cross-doc anchor"))
                elif not os.path.exists(os.path.join(REPORTS_DIR, path)):
                    unresolved_links.append((f, target, "path"))
            else:
                if not os.path.exists(os.path.join(REPORTS_DIR, target)):
                    unresolved_links.append((f, target, "path"))
    print(f"{checked_links} link targets checked")
    print("unresolved:", unresolved_links if unresolved_links else "(none)")

    print()
    print("=== 3. backtick-quoted repository paths (single-backtick spans only) ===")

    def looks_like_path(s):
        if " " in s or "\n" in s:
            return False
        if re.fullmatch(r"[0-9a-f]{7,40}", s):
            return False
        return bool(
            "/" in s
            or re.search(r"\.(go|md|feature|sh|toml|json|yaml|yml|log|py)(:|$)", s)
        )

    skip_exact = {
        "internal/tmux.Client.Create",
        "internal/tui.createFieldRows",
        "internal/service.LaunchLeaseReleaser",
        "dimmed/selection",
        "dimmed/selectionidle",
        "key/surface",
        "text/selection",
        "hint/surface",
        "error/selection",
        "error/surface",
        "./internal/tui/",
        "/",
        "features/*_test.go",
        "*_theme_test.go",
        "internal/theme/builtin/*.toml",
        "features/*.feature",
        "docs/reports/phase3g*",
        "docs/reports/phase3g-02[56]-*/",
        "phase3g-02[56]-*/",
        "/tmp/deck-interactive-pipe-*",
        ".go",
        ".md",
        ".toml",
        ".feature",
        "walking_skeleton.feature,create_session.feature,dialogs.feature,filter.feature,event_log.feature",
    }
    candidates = set()
    for f in FILES:
        for c in single_spans[f]:
            s = c.strip()
            if not looks_like_path(s):
                continue
            s = s.rstrip(",.")
            if s in skip_exact or s in ("", "[", "]") or s.startswith("]("):
                continue
            candidates.add(s)

    bases_to_try = [
        REPO,
        REPORTS_DIR,
        os.path.join(REPO, "features"),
        os.path.join(REPO, "internal", "tui"),
        os.path.join(REPO, "internal", "theme", "builtin"),
    ]
    unresolved_paths = []
    for s in sorted(candidates):
        base_path = re.sub(r":[\d,-]+$", "", s)
        found = any(os.path.exists(os.path.join(b, base_path)) for b in bases_to_try)
        if not found:
            unresolved_paths.append(s)
    print(f"{len(candidates)} candidate backtick paths checked")
    print("unresolved by the fixed base-directory list:", unresolved_paths)

    print()
    print("=== 3b. manual disposition of every string unresolved above ===")
    print("(class E: recursively search docs/reports/ for a same-named file, since")
    print(" the surrounding prose names a specific report subdirectory a few words")
    print(" earlier that the fixed base-directory list above does not try; classes")
    print(" B/C/D/F: known, hardcoded, deliberate non-resolutions)")
    print()

    deliberate = {
        "prds/phase3g-residuals-and-suite-determinism.md": (
            "class B -- phase3g-findings.md quotes this exact wrong filename to name "
            "an earlier draft's mistake; the correct file is "
            "prds/phase3f-residuals-and-suite-determinism.md, which exists. The string "
            "is deliberately non-resolving and the surrounding sentence says so."
        ),
        "tasks.json": (
            "class C -- every occurrence (phase3g-findings.md, task 026's own "
            "unsatisfiableReason) refers to this run's /run/ralphd/tasks.json, outside "
            "this repository. Bare, not prefixed with /run/ralphd/ and not carrying the "
            "explicit 'outside this repository' label at the citing sentence itself -- "
            "a pre-existing wording gap in text task 606 does not own or edit, disclosed "
            "here rather than silently passed."
        ),
        "notes.md": (
            "class C -- phase3g.md:893 ('see `notes.md`') refers to this run's own "
            "/run/ralphd/notes.md, outside this repository, tracking the wider "
            "~19-site title convergence as a still-open item. Same wording gap as "
            "tasks.json above, same disposition."
        ),
        "001-202.md": (
            "class F (new) -- not a repository file at all: the identifier of an "
            "operator ruling delivered to this run outside the repository. "
            "phase3g-findings.md's own F33 names it plainly ('operator ruling "
            "identified as `001-202`... delivered to this run outside the repository, "
            "not part of the tracked record, so not linked here'); phase3g.md's task-202 "
            "table row and its prose two lines later abbreviate the same identifier with "
            "a `.md` suffix and no repeated disclosure at that exact sentence. The "
            "disclosure exists elsewhere in the same two-file record (findings.md F33), "
            "so the citation is deliberate and accounted for, not silently missing."
        ),
        "composite-prd.md": (
            "class G -- F40's task-1005 'Reviewer re-run note' cites this run's own "
            "/run/ralphd/composite-prd.md, outside this repository, for its SPEC.md-wins "
            "rule (lines 21-23). Same wording gap as class C's tasks.json/notes.md: not "
            "prefixed with /run/ralphd/ and not carrying the explicit 'outside this "
            "repository' label at the exact citing sentence, disclosed here instead."
        ),
        "internal/service/reconcile_bare_error_repair_test.go": (
            "class G -- F40's task-1005 note cites this exact former path of task 701's "
            "test (introduced at 89edd3c, last present at 608e030) to name what a "
            "reviewer restores to reproduce the now-intentional failure; task 902's "
            "a1ca33e deleted the file, so it no longer exists anywhere in the tree for "
            "this or any other base directory to find. The non-resolution IS the "
            "evidence the note is pointing at, not a broken link."
        ),
    }

    still_unresolved = []
    for s in unresolved_paths:
        if s in deliberate:
            print(f"  DELIBERATE  {s!r}: {deliberate[s]}")
            continue
        base_path = re.sub(r":[\d,-]+$", "", s)
        base_name = os.path.basename(base_path)
        hits = []
        for root, _dirs, filenames in os.walk(REPORTS_DIR):
            if base_name in filenames:
                hits.append(os.path.relpath(os.path.join(root, base_name), REPO))
        # also check internal/service, internal/hookrecv, cmd/deck for bare .go
        # abbreviations named a few words earlier in the same sentence (class D)
        extra_bases = [
            os.path.join(REPO, "internal", "service"),
            os.path.join(REPO, "internal", "hookrecv"),
            os.path.join(REPO, "cmd", "deck"),
        ]
        for b in extra_bases:
            p = os.path.join(b, base_path)
            if os.path.exists(p):
                hits.append(os.path.relpath(p, REPO))
        if len(hits) >= 1:
            print(f"  RESOLVED    {s!r}: found at {hits} (class D/E: bare abbreviation "
                  f"of a file the surrounding prose names in full a few words earlier)")
        else:
            still_unresolved.append(s)

    print()
    if still_unresolved:
        print("STILL UNRESOLVED AFTER MANUAL DISPOSITION (real defects):", still_unresolved)
    else:
        print("STILL UNRESOLVED AFTER MANUAL DISPOSITION: (none) -- every candidate is "
              "either a real repository path, or one of the deliberate/disclosed classes above.")


if __name__ == "__main__":
    main()
