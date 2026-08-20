# tmux embedded-preview spike

## Decision

**Recommendation: keep the 1-second `capture-pane -e` polling architecture exactly as
`SPEC.md` §11 specifies. Do not attach an embedded tmux client in v1.**

A cell-grid implementation is technically possible and dramatically lowers update latency,
but it does not preserve deck's more important geometry guarantee. With deck's current
`window-size latest` policy, making a useful 45×22 preview client changes the shared pane to
45×22 and sends the real program `SIGWINCH`; read-only mode does not prevent this. The only
measured way to keep the real 120×40 pane untouched is to switch to `window-size manual` and
pin it, which makes ordinary smaller `↵` attaches cropped/pannable rather than allowing the
latest real terminal to govern the window. The embedded path also used **13.118280× CPU** and
**9.699860× peak RSS** in the 30-second synthetic benchmark. Its latency advantage
(0.060538 ms median versus 496.904196 ms) and deterministic composition do not outweigh the
geometry regression, ordinary-attach behavior change, dependency/lifecycle complexity, and
continuous resource cost.

This was a throwaway implementation. No product code or planning/specification file was
changed. All paths below are relative to the repository root, and all retained raw evidence
is intentionally under the ignored `.spike-preview/` tree.

## Reproduction and evidence map

The retained run used the repository CI image: Go 1.25.13 and tmux 3.5a. Run every
experiment through `ci/run.sh`:

```sh
ci/run.sh sh -c 'cd .spike-preview && go test ./...'
ci/run.sh sh -c 'cd .spike-preview && geometry/run.sh'
ci/run.sh sh -c 'cd .spike-preview && pinned/run.sh'
ci/run.sh sh -c 'cd .spike-preview && ./conformance/run.sh'
ci/run.sh sh -c 'cd .spike-preview && benchmark/run.sh'
ci/run.sh sh -c 'cd .spike-preview && latency/run.sh'
ci/run.sh sh -c 'cd .spike-preview && determinism/run.sh'
ci/run.sh sh -c 'cd .spike-preview && failures/run.sh'
ci/run.sh go test ./...
```

These commands are implemented by the named scripts rather than by undocumented manual
steps. Each tmux script chooses uniquely named `deck-spike-*` private sockets and records a
cleanup check. The evidence entry points are:

| Question | Summary / raw evidence |
|---|---|
| load generator and 45×22 composition | `.spike-preview/README.md`, `.spike-preview/preview/`, `.spike-preview/evidence/composition/go-test.log` |
| geometry matrix | `.spike-preview/geometry/README.md`, `.spike-preview/evidence/geometry/results.tsv`, `.spike-preview/evidence/geometry/raw/` |
| pinned window | `.spike-preview/pinned/README.md`, `.spike-preview/evidence/pinned/results.tsv`, `.spike-preview/evidence/pinned/raw/` |
| emulator conformance and provenance | `.spike-preview/conformance/README.md`, `.spike-preview/evidence/conformance/` |
| resource benchmark | `.spike-preview/benchmark/README.md`, `.spike-preview/evidence/benchmark/` |
| sentinel latency | `.spike-preview/latency/README.md`, `.spike-preview/evidence/latency/` |
| 80×24 determinism | `.spike-preview/determinism/README.md`, `.spike-preview/evidence/determinism/` |
| failure modes | `.spike-preview/failures/README.md`, `.spike-preview/evidence/failures/` |
| final product suite | `.spike-preview/evidence/product-suite/` |

The no-credentials generator enters/exits the alternate screen and draws full-screen SGR
colour, box drawing, `界`, a moving spinner, and a scrolling region. `SPIKE_PREVIEW_HZ`
controls live redraws; `SPIKE_PREVIEW_MODE=fixed` produces byte-identical input. Its tests
also exercise `SIGWINCH` telemetry.

## Geometry: read-only clients still participate

Each matrix case kept an ordinary read-only 120×40 PTY and a read-only 45×22 control-client
PTY attached concurrently. `control`, `during`, and `after` mean before preview attach,
while attached, and after detach. `C/D/A dims` are measured window dimensions. `C/D/A hash`
is the SHA-256 of `capture-pane -e` after normalizing only the deliberately moving frame and
spinner counters; the unnormalized capture and hash for every phase are retained in
`evidence/geometry/results.tsv` and `raw/*.capture-e.bin`. `C/D/A winch` is the generator's
cumulative `SIGWINCH` count.

| window-size | aggressive-resize | `refresh-client -C 45x22` | C/D/A dims | C/D/A normalized `capture-pane -e` hash | 120×40 rendering changed during? | C/D/A winch |
|---|---:|---:|---|---|---:|---|
| latest | off | no | 120×40 / 120×40 / 120×40 | `258b9f…f10a` / `258b9f…f10a` / `258b9f…f10a` | no | 1 / 1 / 1 |
| latest | off | yes | 120×40 / **45×22** / 120×40 | `258b9f…f10a` / **`5355b2…4ede`** / `258b9f…f10a` | **yes** | 1 / 2 / 3 |
| latest | on | no | 120×40 / 120×40 / 120×40 | `258b9f…f10a` / `258b9f…f10a` / `258b9f…f10a` | no | 1 / 1 / 1 |
| latest | on | yes | 120×40 / **45×22** / 120×40 | `258b9f…f10a` / **`5355b2…4ede`** / `258b9f…f10a` | **yes** | 1 / 2 / 3 |
| manual | off | no | 120×40 / 120×40 / 120×40 | `258b9f…f10a` / `258b9f…f10a` / `258b9f…f10a` | no | 3 / 3 / 3 |
| manual | off | yes | 120×40 / 120×40 / 120×40 | `258b9f…f10a` / `258b9f…f10a` / `258b9f…f10a` | no | 3 / 3 / 3 |
| manual | on | no | 120×40 / 120×40 / 120×40 | `258b9f…f10a` / `258b9f…f10a` / `258b9f…f10a` | no | 3 / 3 / 3 |
| manual | on | yes | 120×40 / 120×40 / 120×40 | `258b9f…f10a` / `258b9f…f10a` / `258b9f…f10a` | no | 3 / 3 / 3 |

The complete hash used above is
`258b9f153ec547ce57bf3e897f7b8df992157f6b96cb9a2722ce3ba147baf10a`;
the resized hash is
`5355b216bf74902aca23e1048964243f1f9283ea2588a4d23e9e0478b2d14ede`.
The untouched combinations are both `latest` rows **without** `-C`, and all four `manual`
rows. That needs an important qualification: on tmux 3.5a a control client starts with a
virtual width of 80 despite its kernel PTY being 45×22. Omitting `refresh-client -C` therefore
does not establish a correctly sized 45×22 tmux client; it only explains why those two
`latest` controls happen not to perturb the window. Once the preview announces its useful
45×22 size, both `latest` combinations resize the real pane. `aggressive-resize` made no
difference in this one-window concurrent-client matrix.

The raw, changing hashes are not hidden: for example every 120×40 control raw hash is
`d98ab75899e83a6c6f1dab2f90793cbbe28ddcaea95b51103ffd4251ebb29f35`, while the
45×22 during hash is `e74d7fba76f582741818ca739b913bf7407c12c925c9507577e2299a0504e4bb`.
The raw 120×40 during/after hashes differ because the generator is intentionally live; the
normalization is what distinguishes animation from a geometry/rendering change.

## Pinned-window alternative and its user-visible cost

The separate pinned experiment used `window-size manual` followed by
`resize-window -x 120 -y 40`. The original ordinary read-only 120×40 client, a read-only
45×22 control preview (`refresh-client -C 45x22`), and an ordinary read-only 80×24 client
were attached together.

* The window and original client remained 120×40. The normalized capture hash stayed
  `258b9f…f10a`; the original rendering did not change and the retained telemetry count was
  one.
* The control preview received the pinned 120×40 pane layout and pane-output protocol. A
  control protocol stream is neither cropped nor letterboxed by tmux. The preview consumer
  must emulate the complete 120×40 grid and choose its own 45×22 crop or scaling policy.
  tmux 3.5a reported `client_width=45` but an empty control-client height; the kernel PTY
  independently remained 45×22.
* The ordinary 80×24 `↵` attach saw an 80×24 viewport into the larger 120×40 window, with a
  recorded tmux viewport offset. This is **cropping/panning, not letterboxing**. It is a
  direct change from §3.3's current latest-active-client behavior.

Thus pinning solves preview interference only by changing the real attach contract for all
users. It is not a free preview-only option.

## Composition and emulator choice

The Bubble Tea spike feeds pane bytes through a terminal emulator cell grid; it never
passes foreign escape sequences through to deck's outer terminal. The composed panel is
45×22 total with rounded `╭╮╰╯` borders and one column of horizontal padding. The automated
test processes 50 changing generator frames and checks all 22 lines at display width 45,
including the border on every line. The retained `go test ./...` output is in
`.spike-preview/evidence/composition/go-test.log`.

Passing pane bytes directly through would let cursor movement, alternate-screen, erase, and
SGR control sequences act on deck's terminal and corrupt its chrome; it is not a viable
implementation. The successful spike uses a grid and emits only composed outer-frame
output.

The same conformance byte stream was independently fed to both candidates. Both build in
the CI image and are MIT licensed (exact license texts are retained).

| candidate | selected source | builds | alt screen | SGR colour | box drawing | EAW-wide placement | maintenance evidence | verdict |
|---|---|---:|---:|---:|---:|---:|---|---|
| `github.com/hinshun/vt10x` | `v0.0.0-20220301184237-5011da428d02` (2022-03-01) | yes | pass | pass | pass | **fail**: `界` at x0 was followed by `┌` at x1 rather than a continuation and x2 | selected source is from 2022; retained repository metadata showed last push 2023-12-06 | reject |
| `github.com/charmbracelet/x/vt` | `v0.0.0-20260816001655-68d539dca504` | yes | pass | pass | pass | pass: width 2, continuation at x1, `┌` at x2 | active Charm `x` monorepo; retained metadata showed push 2026-08-16 | **choose if embedded** |

**Named choice: `github.com/charmbracelet/x/vt`.** It passed all measured capabilities,
exposes alternate-screen state and cell/grapheme widths, and has materially fresher
maintenance. `vt10x`'s rune-only cell model already failed the required wide-cell placement.
The provenance snapshots are run evidence, not a promise that a future dependency revision
will behave identically.

Dependency gotcha: released Bubble Tea v1.3.10 was incompatible with the current
`x/vt`/`x/ansi` set used by the spike, so the ignored module pins compatible Bubble Tea and
Charm pseudo-versions. Product adoption would require deliberate version reconciliation and
normal dependency review; copying only the `x/vt` requirement into deck is not sufficient.

## Continuous resource cost

The generator ran at **100 redraws/s**, the highest tested rate, for separate fixed
30-second windows. Both strategies used a 41×20 live grid (inside a 45×22 panel).
`measure.py` used `os.wait4`: CPU is cumulative user+system time for the measured command
tree, and Linux `ru_maxrss` is the maximum resident set of any measured process in KiB,
not aggregate simultaneous RSS.

The baseline scope was the persistent poller and each once-per-second `tmux capture-pane -e`
child. The embedded scope was `timeout` plus `benchpreview`, which fed every `pipe-pane`
chunk through `x/vt` and composed the same fixed chrome. tmux, the load generator, and the
small forwarding process were outside **both** measured scopes; composed output was
discarded.

| metric | 1 s capture baseline | embedded emulator | embedded / baseline |
|---|---:|---:|---:|
| elapsed seconds | 30.002625 | 30.007483 | 1.000162× |
| user CPU seconds | 0.044856 | 1.129839 | 25.188135× |
| system CPU seconds | 0.055203 | 0.182763 | 3.310744× |
| **total CPU seconds** | **0.100059** | **1.312602** | **13.118280×** |
| **peak RSS KiB** | **11,408** | **110,656** | **9.699860×** |

These are synthetic process-scope measurements, not whole-machine claims. They nevertheless
measure the incremental long-running preview paths on the same workload and show an
unfavorable continuous-cost ratio.

## Sentinel latency

Each strategy had ten independent fresh private-server samples. The start timestamp is when
`tmux pipe-pane` observed the byte chunk containing the newly painted persistent sentinel;
it is deliberately **not** generator emission time or `send-keys` time. Capture visibility
was recorded only after the once-per-second poll returned the sentinel. Embedded visibility
was recorded only after `x/vt` consumed the bytes and the complete composed 45×22 string
contained it. Units are milliseconds.

| sample | capture polling | embedded composition |
|---:|---:|---:|
| 1 | 946.718421 | 0.060376 |
| 2 | 847.000972 | 0.054684 |
| 3 | 747.821693 | 0.075632 |
| 4 | 648.000029 | 0.060931 |
| 5 | 546.325175 | 0.075979 |
| 6 | 447.483217 | 0.058177 |
| 7 | 346.627894 | 0.039759 |
| 8 | 247.956414 | 0.060701 |
| 9 | 146.158858 | 0.064334 |
| 10 | 46.973673 | 0.047880 |
| **min / median / max** | **46.973673 / 496.904196 / 946.718421** | **0.039759 / 0.060538 / 0.075979** |

The embedded result is decisively faster. The capture distribution is the expected phase
relationship to a one-second tick rather than evidence of a confused emission clock.

## 80×24 determinism

The fixed sequence-zero generator frame was rendered into a complete 80×24 side-by-side
frame (35-column sidebar panel, 45-column preview panel) ten times. Every run created a
fresh `x/vt` emulator. All ten outputs were byte-identical and had SHA-256:

```text
20c1d94bc55ff70e830c52b90b138539c170d2ea736dd97e46ed25bb555ea1a8
```

The verdict is therefore **pass** for controlled fixed input. The ten frames and hashes are
retained in `.spike-preview/evidence/determinism/`; `byte-differences.tsv` is empty apart
from its header because no mismatch occurred.

Even though no remedy was triggered by this run, an embedded product golden must inject a
deterministic fake/fixed pane and wait for a defined complete frame before assertion. A
live agent pane cannot be part of a byte-golden: cursor timing, animation, and agent output
are external nondeterminism. Excluding the preview region would make the chrome/layout
golden stable but would give up coverage of cell conversion, clipping, SGR, wide glyphs,
and the preview borders. The fixed fake is the required remedy if embedded architecture is
chosen; use a separate live scenario for transport behavior.

## Failure modes

All cases ran under a host wrapper that emitted `HOST_CHROME_SURVIVED` only after the tmux
client returned. This is an observable transport-level stand-in for deck regaining control;
it is not a claim that production Bubble Tea integration already exists.

1. **Session killed with default `detach-on-destroy on`:** `kill-session` forced the
   attached preview client out (`%exit` in the retained control transcript). The host
   wrapper survived. The shared window was 120×40 before destruction; generator
   `SIGWINCH` count was zero.
2. **Pane exits non-zero with `remain-on-exit failed`:** exit status 7 left
   `pane_dead=1 pane_dead_status=7 pane_current_command=sleep`. The client stayed attached
   to the retained dead pane until explicitly detached; the host wrapper then survived.
   A product transport must recognize dead-pane metadata rather than wait forever for
   further output.
3. **Missing preview target:** tmux exited 1 with the exact error
   `can't find session: definitely-missing`; no client attached and the host path
   continued. This must render an in-panel unavailable state, not terminate deck.
4. **Outer terminal resized during preview:** resizing the preview PTY from 45×22 to 70×30
   changed the latest-policy shared window from 45×22 to 70×30 and produced two observed
   `SIGWINCH` records. The client remained attached and the host survived explicit detach.
   Resize propagation therefore amplifies, rather than fixes, the geometry problem.
5. **Two deck-like clients preview the same session:** both read-only control clients
   coexisted. Geometry went 120×40 → 45×22 → 45×22 and the generator recorded one
   `SIGWINCH`; both wrappers survived detach. Read-only prevents input, not geometry
   participation, and a second preview does not create an independent pane grid.

Every server was cleaned up; see each experiment's `socket-cleanup.txt`.

## Interactivity: possible, deliberately not implemented

An embedded cell grid could technically accept keys by translating focused Bubble Tea key
events and sending them to tmux (through a writable client or a separately authorized
`send-keys` command). That does **not** make it safe or desirable. The preview transport in
this spike is read-only, and no input path was built.

At minimum, an interactive design would need all of the following wrong-session safeguards:

* bind focus to an immutable socket/session/pane identity captured when the preview is
  selected, not merely the currently highlighted row name;
* immediately before every dispatch, atomically re-resolve and verify that identity and a
  selection generation, and reject on rename, pane replacement, reconnect, death, or stale
  generation;
* cancel buffered/queued key events whenever selection, focus, target, or transport changes;
* show the exact target and an unmistakable input-focus state, default to no input, and
  never replay keys after reconnect;
* preserve tmux's literal/control-key semantics deliberately and audit sends.

That would be a product change, not an implementation detail. §11 currently says the
preview is never interactive and `↵` obtains a real terminal. §11.1 intentionally permits
`s` only from `idle`, only as a single line, with adapter support and no `--force`; making
arbitrary preview keystrokes possible would bypass those state and prompt protections.
It would require rewriting §11 and §11.1, keymap/focus/footer/help behavior, mouse behavior,
audit semantics, and black-box wrong-target scenarios. This report does not recommend or
implement it.

## Additional gotchas

* A PTY size is not necessarily a control client's tmux virtual size. On tmux 3.5a the
  45×22 control PTY initially presented virtual width 80 until `refresh-client -C`.
* tmux 3.5a left control-client height metadata empty in the pinned case. Keep independent
  PTY geometry evidence rather than trusting one format field.
* `attach -r` means input read-only; it does not opt the client out of window-size policy.
* `aggressive-resize` on/off did not protect `window-size latest` once the preview issued
  `refresh-client -C 45x22`.
* A pinned control stream is the full pane protocol, not a tmux-cropped miniature. Deck
  would own viewport/crop policy and emulator memory for the larger grid.
* Alternate-screen output cannot be safely inserted as a string into Bubble Tea chrome.
  Only a cell-grid snapshot may be composed into the outer frame.
* Pane-output streaming needs initial-screen synchronization and explicit lifecycle/error
  states; a stream alone is not equivalent to `capture-pane`'s current snapshot semantics.
* Multiple previewers share the same tmux window and therefore the same geometry.
* `detach-on-destroy`, missing targets, and retained dead panes return three different
  transport outcomes that must converge on a stable in-panel state without killing deck.
* The synthetic benchmark did not use an authenticated Claude process. That optional
  confirmation was not required and no credentials were sought.

## Proposed specification and plan changes (not applied)

### Recommended capture-polling option

`SPEC.md` needs **no architectural change**: retain §3.2's `window-size latest` and
`aggressive-resize on`, §3.3's latest-active-client attach behavior, and §11's exact
“pane capture with escapes preserved, 1 s tick, selected row only; no embedded PTY
emulator” contract. When Phase 2b turns the prose into tests, clarify without changing
behavior that foreign escapes are interpreted into the harness/product cell model before
chrome composition and never blindly passed through to the outer terminal. Keep
`DECK_PREVIEW_MS` in §13.1 and the capture-history scrolling language in §§11.3/11.8.

For `docs/PLAN.md`, mark the preview spike done with this capture recommendation. Keep the
Phase 2b preview scope as a selected-row `capture-pane -e` poll, add assertions that preview
polling creates no attached tmux client and cannot change window dimensions or produce
`SIGWINCH`, and retain the 80×24 35/45 golden using deterministic captured fixture bytes.
No emulator dependency, streaming-client lifecycle work, pinned-window behavior, or
interactive key path should enter Phase 2b.

### Rejected embedded-emulator option

If the operator chooses embedded despite this recommendation, the change cannot be limited
to one sentence in §11:

* **`SPEC.md` §§3.2–3.3:** replace `window-size latest` with a defined manual pinning and
  ownership policy; specify when/how the pin follows the primary real attach, define
  45×22 preview cropping into the full grid, and explicitly document that smaller ordinary
  attaches see a cropped/pannable viewport. Define concurrent previews and resize behavior.
* **§11 and §§11.2–11.3/11.8:** replace capture polling/history with a selected-row,
  noninteractive read-only control stream rendered through `charmbracelet/x/vt`; define
  initial snapshot synchronization, frame/coalescing rate, crop/scroll behavior, dead and
  missing target states, teardown, focus, and the resource budget. Keep `↵` as the only
  interactive terminal path.
* **§13.1–13.2:** replace or redefine `DECK_PREVIEW_MS`, add a deterministic fixed fake-pane
  source and transport controls, and require geometry, dual-preview, kill, missing-target,
  dead-pane, resize, wide-cell/SGR/alternate-screen, and no-escape-leak black-box scenarios.
  The 80×24 golden must use that fake pane rather than a live process.
* If interaction is also desired, rewrite §11.1 and the §11 keymap/help/footer/mouse and
  audit contracts around the safeguards listed above; it cannot coexist honestly with the
  current “never interactive” and narrow `s` contract.

For `docs/PLAN.md`, Phase 2b would need explicit work items for dependency reconciliation,
`x/vt` cell conversion, snapshot-plus-stream synchronization, manual-window ownership and
viewport UX, transport supervision, cleanup on every Bubble Tea exit path, all five failure
modes, concurrent previewers, deterministic fake-pane golden coverage, and CPU/RSS
acceptance budgets. It would also need to call out the deliberate §3.3 ordinary-attach
regression and budget materially more implementation and harness work. The spike row would
be marked done with an embedded decision rather than merely “running.”

## Final product-suite proof

After the experiments, the exact command was run once:

```sh
ci/run.sh go test ./...
```

It exited 0. The complete command, output, and status are retained in
`.spike-preview/evidence/product-suite/{command.txt,output.log,transcript.log,exit-status.txt}`.
The package output includes all product packages and ends with `exit status: 0`.

At that point `git diff --name-only` contained only `.gitignore`; the protected-path check

```sh
git diff --name-only -- SPEC.md docs/PLAN.md prds internal cmd features ci
```

was empty. The retained proof is
`.spike-preview/evidence/product-suite/workspace-diff.txt`. This report is the sole lasting
spike document; `.spike-preview/**` remains ignored evidence.
