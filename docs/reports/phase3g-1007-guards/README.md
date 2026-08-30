# Task 1007 — guard re-verification at the run's true final sha

Guards (a)–(f) in `guard-a-*.log` … `guard-f-*.log` were first run **at HEAD
`11a9c9c8bdbfefcb0c2125c102a0cee6be89d8f9`** (== `origin/main` before this task's
first commit was created) and then re-run verbatim **at the pushed bundle sha
`2919df2bed43129a4d0110952930f85b1a07a7ac`** — see
[`reverify-at-2919df2.log`](reverify-at-2919df2.log) and the section "Re-verification
at the pushed bundle sha" at the end of this file. Guard (g) is recorded twice:
before this task's first commit in
[`guard-g-clean-head-codediff.log`](guard-g-clean-head-codediff.log) and **after that
commit was pushed** in [`guard-g-postpush.log`](guard-g-postpush.log). Every guard is
scoped to the run range `1cfbd5a..HEAD` (`1cfbd5a` is the run's own base sha — never a
full-history claim; a full-history claim of this exact kind is what sank task 113). No
guard log is hand-edited: each file here is the unedited output of the exact command it
quotes, with the command's exit status captured in the same shell call. None of the seven
guards below found anything that needed a `docs/reports/phase3g-findings.md` entry — all
seven hold clean.

Task 1007 is delivered by two commits: the bundle commit
`2919df2` and this follow-up (fix-forward per the standing rules, which forbid amending or
rewriting published history) whose only purpose is to publish output that could not exist
when `2919df2` was written — the state of the repository *after* `2919df2` was pushed.

## (a) Protected paths — `guard-a-protected-paths.log`

`git diff 1cfbd5a..HEAD -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md` prints nothing.
`git log --all --oneline 1cfbd5a.. -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md` lists
exactly two commits over the run range: `b69b5ba` (steering-018's licensed SPEC.md
§11.8 amendment, task 205) and `2d61993` (its forward-revert, task 909). That pair
is an edit plus its own revert — net zero content change to every protected path
across the run range — and this guard makes **no claim about protected-path history
before `1cfbd5a`** (the run's own base sha is where the range starts; anything
earlier is out of scope for this run by construction).

## (b) `features/godog_test.go`'s `defaultTags` — `guard-b-godog-test.log`

`git show 1cfbd5a:features/godog_test.go | grep 'const defaultTags'` and the same
command against `HEAD` both print the byte-identical line
`const defaultTags = "~@real-agents && ~@nightly"`. The file's only diff in
`1cfbd5a..HEAD` (also captured in the log) adds two `registerSteps` call lines
inside `initializeScenario` (`registerTerminalRepairFieldRouteSteps`,
`registerInteractivePipeLeakSteps`) — new feature-step registrations for new
scenarios, not a tag-authority edit.

## (c) No scenario removed or retagged; every added `t.Skip` justified — `guard-c-scenarios-and-skips.log`

`git diff -U0 1cfbd5a..HEAD -- '*.feature'` shows four removed `Scenario:` title
lines. Each was reviewed individually against the same file at both shas (title,
`@requirement-*` tag if any, and the file's total scenario count): all four are
same-position **renames** whose title text was updated to match a landed
behaviour change in the same hunk (arrow-key rebinding, a corrected/strengthened
assertion, or the R76 self-heal-race rewrite already recorded in this run's R76
section and finding F40) — no `@requirement-*` tag changed on any of them, and no
feature file's total scenario count dropped. The log spells out each of the four
with its file, before/after title, tag (or "untagged, confirmed at both shas"),
and count evidence.

Five `t.Skip` calls were added across `internal/tui/*_theme_test.go` in the range,
all textually near-identical ("this theme's key and hint tokens share a colour;
the ... distinctness assertion below would be vacuous") — these are exactly the
"conditional theme-colour `t.Skip`" class the standing rules name explicitly as
one of the things a sweep report must never silently omit; they fire only when a
specific theme's palette happens to collide a key token's colour with a hint
token's colour, at which point the distinctness assertion they guard would pass
for the wrong reason. No other `t.Skip` was added in range; the pre-existing,
opt-in `TestI1KeystrokeDropReproduction` driver (`features/i1_repro_test.go`) is
untouched by this diff range (confirmed empty `git diff 1cfbd5a..HEAD` on that
file).

## (d) `internal/theme/builtin/` — `guard-d-theme-builtin.log`

`git diff --stat 1cfbd5a..HEAD -- internal/theme/builtin/` prints nothing. No
theme palette changed in this run.

## (e) No diff touches a session's cwd handling — `guard-e-cwd-handling.log`

`git diff 1cfbd5a..HEAD -- '*.go' | grep -n 'CWD\|-c '` has ~204 hits, reviewed in
full in the log. All but one are either (i) new test/step-definition fixtures
supplying a `CWD:`/`cwd` value the same way pre-existing ones already did, or
(ii) the create-modal's own `createCWD*` UI field state (prefill, ghost
completion, recent-cwd cycling) together with its step-definition plumbing —
none of which reads, writes or otherwise changes how a *session's* cwd is
resolved or passed to tmux. The one hit that touches real session-launch code
is `internal/tmux/tmux.go`'s `Client.Create` (task 501's env-mirroring race fix):
it moves new `-e KEY=VALUE` flags earlier in the args slice and folds a
previously-separate `set-environment` call into the same `new-session`
invocation, but the `-c launch.CWD` pair itself is unchanged in value, meaning
and position relative to the command name — only unrelated `-e` flags moved
around it. This does not touch `launch.CWD`, the `-c` argument's value, or any
cwd path, so it does not trip the standing prohibition. The sole non-`CWD`
`-c ` match is a code comment quoting a test fixture's shell command
(`` `sh -c 'exit 1'` ``), not a deck-issued tmux flag.

## (f) Citation sweep — `guard-f-citation-sweep.log`

`python3 docs/reports/phase3g-813-guards/citation_sweep.py`, run from `/workspace`,
**exits 0**. Its own manual-disposition pass leaves exactly the run's known,
disclosed false-positive classes unresolved (`001-202.md`, `composite-prd.md`,
`internal/service/reconcile_bare_error_repair_test.go`, `notes.md`, `tasks.json`,
`prds/phase3g-residuals-and-suite-determinism.md`, `/run/ralphd/approaches/NN/tasks.json`,
`9/10`) — no new unresolved citation.

## (g) Clean tree / HEAD == origin/main / no code diff since the final code sha — `guard-g-postpush.log` (post-push) and `guard-g-clean-head-codediff.log` (pre-commit)

**Post-push capture — `guard-g-postpush.log`.** Taken from `/workspace` after the
bundle commit `2919df2` was pushed to `origin/main`, this log holds the required pair
as raw output, no property substituted for it: `git status --porcelain` prints nothing
(clean tree — the guard bundle is tracked now, so it is no longer `??`);
`git rev-parse HEAD` and `git rev-parse origin/main` both print
`2919df2bed43129a4d0110952930f85b1a07a7ac`, with
`test "$(git rev-parse HEAD)" = "$(git rev-parse origin/main)"` printing `EQUAL`;
`git ls-remote origin refs/heads/main` shows the same sha on the remote itself, so the
equality is not a stale local ref; and
`git diff --stat a5f8f6b..HEAD -- '*.go' '*.feature' '*.sh' '*.toml' go.mod go.sum`
(`a5f8f6b`, task 1001, is the run's final CODE sha) still prints nothing. That log lives
in this follow-up commit for one mechanical reason: the output of a check run *after* a
commit is pushed cannot be a file inside that same commit's tree — no property is being
substituted for the check, the check was really run and its output is quoted above and in
the log. This follow-up commit is itself pushed to the same branch as a fast-forward, and
its own post-push re-check of the same three commands is recorded in this run's handoff
notes, outside this repository, since by the same construction it cannot be a file inside
itself.

**Pre-commit capture — `guard-g-clean-head-codediff.log`.** Captured **before** this
task's first commit was created, at HEAD `11a9c9c`:
`git status --porcelain` shows only this task's own not-yet-tracked
`docs/reports/phase3g-1007-guards/` (expected — that directory becomes tracked,
not dirty, the moment this task's commit lands); `git rev-parse HEAD` and
`git rev-parse origin/main` both resolve to `11a9c9c8bdbfefcb0c2125c102a0cee6be89d8f9`;
`git diff --stat a5f8f6b..HEAD -- '*.go' '*.feature' '*.sh' '*.toml' go.mod go.sum`
(`a5f8f6b`, task 1001, is the run's final CODE sha) prints nothing.

**Why the bundle commit `2919df2` could not carry its own post-push pair (history,
superseded by `guard-g-postpush.log` above):** a commit cannot contain, inside a file that is part of
its own tree, the literal hash value that hashing that very tree (plus this
commit's parent/message/author/committer) will produce — writing that value into
the file changes the tree, which changes the hash, which invalidates the value
just written. This run has already hit exactly this regress twice on the
identical guard: `docs/reports/phase3g-813-guards/` needed a `72c936d` addendum
commit and then a further `9e86f4c` round-2 addendum to name its own correction's
sha, each admitting outright ("A commit cannot quote its own sha, so the
correction commit's clean-tree and HEAD-equals-origin/main pair needs a follow-up
to be citable at all"); `docs/reports/phase3g.md`'s approach-08 close-out section
spiralled through *three* further self-naming addenda (commits B, C/D, E/F) for
the same reason. Task 1007 asks for **one** commit, which forecloses that spiral
by construction — so `2919df2` argued the property instead
(pushing an already-fully-formed commit is a plain fast-forward of `main`, after which the
tree is clean and `HEAD` equals `origin/main`), following
`docs/reports/phase3g-606-guards/README.md`'s guard (c). **That is no longer what this
bundle rests on:** the pair is now published as real output in `guard-g-postpush.log`, one
commit later, which costs exactly one follow-up commit and no spiral — this follow-up
quotes `2919df2`'s hash, not its own.

## Re-verification at the pushed bundle sha — `reverify-at-2919df2.log`

All of guards (a)–(f)'s commands were re-run from `/workspace` at HEAD `2919df2`
(clean tree, == `origin/main`) and every result is unchanged from the `11a9c9c`
capture, as expected — `2919df2` is a docs-only commit: (a) the protected-path diff
prints nothing and the `--all` log still lists exactly `2d61993` and `b69b5ba`;
(b) `const defaultTags = "~@real-agents && ~@nightly"` at both `1cfbd5a` and `HEAD`,
with the same two `registerSteps` additions as the file's only in-range diff;
(c) the same four renamed `Scenario:` title lines, the same five theme-colour
`t.Skip` additions, and an empty diff for `features/i1_repro_test.go`; (d) the
`internal/theme/builtin/` diff prints nothing; (e) the same 204 `CWD`/`-c ` hits
(count captured in the log next to the full listing), reviewed in
`guard-e-cwd-handling.log`; (f) `citation_sweep.py`, run unpiped so the captured exit
status is the script's own, exits 0 leaving only `/run/ralphd/approaches/NN/tasks.json`
and `9/10` — the two disclosed false-positive classes (neither is a repository path: the
first is a run-state file outside this repository, quoted here only as the sweep's own
output token, the second is a bare stability ratio the sweep's path heuristic misreads).
