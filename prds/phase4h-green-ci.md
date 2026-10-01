# Phase 4h — Green CI: every run on main is green, nightly path included

## What the operator gets

GH #57: **the Actions tab on `main` is green.** Every push, schedule and `workflow_dispatch` run
of `ci` concludes `success`. That includes the `-race` suite and the nightly stability loop, and
it holds without leaning on the per-scenario rerun. Operator, verbatim: *"we want GREEN CICD.
constant red CICD is meaningless."*

Phase 4g (#50) removed the harness-caused red: skipped publisher runs, cancellations, `-race`
budget assertions and Pages deploy blips. It finished `verified-with-residuals` at `a192accf7d`,
and the Actions tab was still red. Its PRD let a nightly-path failure pass as "advisory" when its
cause was named. **This PRD has no advisory path.** A flaky scenario is a defect, and naming it
does not excuse it.

#57's body is the evidence. Read it before starting. Where the issue and this PRD disagree, this
PRD wins. Where either disagrees with `SPEC.md`, SPEC wins, and the disagreement is a finding.

## The evidence at `a192accf7d`

| run | event | first-attempt features failures (`ci/suite.sh` "rerunning failed scenario alone") | result |
|---|---|---|---|
| 36798676638 | workflow_dispatch | `attach_scroll.feature:11` (failed its solo rerun too), `harness.feature:212`, `no_leak_scan.feature:17` | failure |
| 36792029158 | workflow_dispatch | `dialogs.feature:143`, `harness.feature:212`, `preview.feature:304`, `resume_failure.feature:9`, `status_recovery.feature:23` (failed its solo rerun too); stability run 2 failed on `profile_side_by_side.feature:8` | failure |
| 36791928734 | push | `profile_side_by_side.feature:8` (rescued by the rerun) | success, flaky |
| 36774969847 (at `a3b906bc3b`) | workflow_dispatch | `agent_session.feature:33`, `harness.feature:212` ×2 | failure |

Scheduled nightlies 36406261401 and 36309016544 (27–28 Sep) were also red. The push runs
36707106029 and 36707120590 were red on the transient-"starting"-frame class.

The `:line` locations are as of `a192accf7d`. Scenarios are identified by **name**; if a line
has drifted, find the scenario by name.

## Ground rules

- **These paths are read-only to this job:** `SPEC.md`, `prds/`, `ci/Dockerfile`, `ci/SPIKE.md`,
  `.github/workflows/release.yml` and `ci/releasegate/`.

  ```sh
  BASE=$(git log --format=%H -1 -- prds/phase4h-green-ci.md)   # the operator's PRD commit
  git log --oneline "$BASE..HEAD" -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md \
    .github/workflows/release.yml ci/releasegate/                # must print nothing
  ```
- **Fix causes, not symptoms.** For each flaky scenario, find out why it fails and fix that:
  - an assertion on a transient that may never be painted;
  - a settle predicate that acknowledges the wrong thing;
  - a one-shot write racing the product's own self-heal;
  - an unsynchronised read of an async counter;
  - or a genuine product bug.

  The following are **not** fixes, and each is a finding:
  - adding or extending retries;
  - adding a sleep;
  - skipping, tagging-out or deleting a scenario;
  - narrowing an assertion until it no longer checks the requirement it names;
  - widening a deadline or poll timeout without a measurement.

  A deadline may be widened only when the run **measures** that it is wrong. Put the observed
  latency distribution on CI, under the build mode in question, in artifacts, and set the new
  value from it. A race-build-only widening goes through `internal/racebuild`, as Phase 4g's did.
- **Product fixes are allowed when the product is the cause.** If a flake is a real deck bug (a
  data race, a lost update, a wrong transition), fix the product. The new behaviour must agree
  with `SPEC.md`. If fixing it would need a SPEC change, petition (§Escape hatch).
- **No keymap, schema or rendering change** unless the root cause of a named flake is one. Then
  state why in the commit message.
- **Never touch the operator's live state.** This covers:
  - `~/.local/share/deck/`, `~/.config/deck/` and `~/.local/state/deck/`. The job never opens,
    reads, copies or creates anything there;
  - the live `tmux -L deck` server, and any `tmux -L deck-*` server. Never touch them, not even
    read-only.

  Every test uses its own temp `HOME`/`XDG_*`/`DECK_HOME` and a private socket.
- **Docker is for `ci/run.sh` and `ci/stability.sh`, and nothing else.**
  - Never remove or kill containers by label: a previous ralphd job on this host SIGKILLed itself
    by sweeping `label=ralphd.run`.
  - No `docker prune`, and no wildcard `rm`/`rmi`.
  - Never signal by pattern: resolve a pid, verify it, then signal that pid.
  - Other runs and the two `deck-ws-*` self-hosted runners live on this host. Never stop, restart,
    reconfigure or re-register a runner.
- **No paperwork.** Do not add report, findings, audit, close-out or "retake" files to the repo,
  and do not add a `docs/DELIVERY-LOG.md` row.
  - The flake inventory, measurements, sweep logs and CI run ids go to `/run/ralphd/artifacts`.
  - Do not close GitHub issues: the operator closes them at release.
- **The CI container has no agent binaries.** Every scenario runs against the `cmd/fake-*` stubs on
  a fixture `PATH`.
- **GitHub scope.** The git credentials are the operator's PAT. The job may:
  - push to `main`;
  - read the Actions API, including run logs and artifacts;
  - start `ci.yml` by `workflow_dispatch` on `main`.

  Nothing else:
  - no other branches, no PRs, no tags, no releases, and no direct push to `gh-pages`;
  - no cancelling, re-running or deleting a workflow run;
  - no change to repository settings, environments, Pages, runners or secrets.

## Scope

- **R168:** every flaky scenario in the inventory is fixed at its root.
- **R169:** a preview-only repaint never acknowledges navigation.
- **R170:** a one-shot DB step cannot lose to Reconcile's self-heal.
- **R171:** `docs/ci.md` and comments stay true.
- **§Definition of done:** the proof is consecutive green nightly-path runs with zero flaky
  records.

All of them are acceptance.

## Materiality rubric

Reject only on substance:

- a requirement's behaviour is absent or wrong in the live code or workflows;
- a test the requirement names:
  - is missing or red;
  - asserts the opposite of the requirement;
  - is a regression test that also passes on the unfixed tree `a192accf7d`;
- **any proof run in §Definition of done is red, or has a non-empty flaky record** (any scenario
  or Go test that needed its rerun);
- a fix from §Ground rules' "not fixes" list: a retry, sleep, skip, narrowed assertion or
  unmeasured deadline widening;
- the spec contradicts the live code or workflows;
- a secret is exposed, in the tree, a workflow log or a published CI report;
- a guard in §Ground rules is broken:
  - a protected path is modified (the audit command prints anything);
  - any access to the operator's deck directories or any `tmux -L deck`/`deck-*` server;
  - a branch, PR, tag or release is created, or `gh-pages` is pushed by hand;
  - a workflow run is cancelled, re-run or deleted by the job;
  - a repository setting, environment, runner or secret is changed.

Everything else verifies, with the gap recorded as a residual note. That covers wording, form,
provenance and process, and **any number or claim in prose outside the spec** (commit messages,
notes, run artifacts, `docs/ci.md`). Prose outside the spec is never a blocking ground.

There is **no advisory list.** One exception: a run is not judged when it never reached the
`suite` step because GitHub failed to provision a runner, or because of an Actions API 5xx. Such
a run neither counts toward the proof nor resets it. Record it and dispatch another. A red
`suite` step always counts.

## Escape hatch

If the run proves that a requirement contradicts `SPEC.md`, or that a scenario cannot be made
deterministic without a SPEC change, it must not edit the spec, delete the scenario or quietly
narrow it. File a petition with:

- the evidence;
- the smallest SPEC or scenario change that would resolve it.

Then notify the operator and carry on with everything the petition does not block. The operator
rules by amendment.

## Notifications

The operator is AFK on Telegram, so use the `notify` hat tool in the same iteration as the work.

Send one message for each of:

- anything that blocks work outright, including a petition;
- the finished flake inventory (§R168);
- each flake class as it is fixed;
- every rejection and cure pass;
- each proof run's result, and whether it reset the count;
- one terminal summary.

Keep each message to a few hundred characters: the notifier refuses a body over 16000 characters
outright. A missed or late send is cured by mentioning it in the next one. It is never a success
criterion and never a finding.

## R168 — every flaky scenario in the inventory is fixed at its root

- **Build the inventory first.** Read the Actions history on `main`: at least the last 40 `ci`
  runs (push, schedule and dispatch), their logs, and their uploaded artifacts.
- List every features scenario that failed first time, and every Go test that `gotestsum`
  reran. Take them from `ci/suite.sh`'s "rerunning failed scenario alone" lines and its
  `flaky-*.txt` files.
- List every stability-run failure too.
- Add this run's own local sweep failures as you find them.

  The inventory goes to artifacts, with each entry's failing step and message. It starts from
  #57's list:
  - the transient "starting" frame: `attach_scroll.feature:11` and
    `profile_side_by_side.feature:8`. A frame asserts `starting` on a shell row, but SPEC §7's
    shell-only fast-forward can promote the row to `running` before that frame is ever painted;
  - `harness.feature:212`: a preview fixture rendered once and then silent;
  - `no_leak_scan.feature:17`;
  - `dialogs.feature:143`;
  - `preview.feature:304`;
  - `resume_failure.feature:9`;
  - `status_recovery.feature:23` (R170);
  - `agent_session.feature:33`, the restart retry from cure `a192accf7d` (R169).
- **Group the inventory by mechanism, and fix each mechanism.**
  - A frame assertion on a transient becomes an assertion on a durable observable: the store's
    recorded transition, an audited event, or the settled state.
  - An async counter is read after a settle, not sampled once.
  - A lost race in the harness is ordered by the event it waits for.
  - A product race is fixed in the product.

  Each scenario still checks the requirement its tag names.
- **Success:**
  - Each mechanism has a deterministic regression test, a Go test or a scenario. The test forces
    the bad interleaving (for example: injected scheduling, a paused Reconcile, a scripted frame
    sequence like R169's probe, or a fake clock). It **fails on `a192accf7d`** and passes after.
    Put the evidence in artifacts: the command, and the failure on the unfixed tree.
  - Every inventory scenario passes 20/20 consecutive solo runs under `-race` and 20/20 under a
    normal build, locally through `ci/run.sh`. The logs go to artifacts.
  - §Definition of done's proof runs show none of them.

## R169 — a preview-only repaint never acknowledges navigation

- `sendNavKeySettled` (`features/navigation_settle_test.go`) waits for "the selected sidebar
  line" to change after a navigation key. A repaint of the **preview pane alone** can change that
  comparison before deck has consumed the key. The caller then sends its next key against the old
  selection: B4, where `R` landed on the group header.
- Cure `a192accf7d` mitigated it with a one-shot select-then-send **retry**. Under this PRD, a
  retry is not a fix.
- **After this phase:**
  - the settle predicate acknowledges only a change in the **sidebar selection itself**: which
    row is selected, scoped to the sidebar column, so preview content can never change the
    predicate's result;
  - the retry added by `a192accf7d` is removed;
  - `agent_session.feature`'s restart scenario and every other caller pass without it.
- **Success:**
  - The reviewer probe below, or an equivalent with the same two subtests, is in `features/` and
    passes. It fails on `a192accf7d`.
  - The restart scenario passes 20/20 solo runs under `-race`, with the retry gone.

```go
// A preview repaint can arrive after writing g but before g is consumed.
// It must not acknowledge the navigation key: R would then hit the header.
func TestReviewPreviewRepaintCannotAcknowledgeNavigation(t *testing.T) {
	for _, previewOnly := range []bool{false, true} {
		name := "selection_change_is_acknowledged"
		if previewOnly {
			name = "preview_only_change_is_not_acknowledged"
		}
		t.Run(name, func(t *testing.T) {
			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			defer writer.Close()
			d := &ScreenDriver{terminal: writer, screen: vt.NewEmulator(90, 5), updated: make(chan struct{}, 1), done: make(chan struct{})}
			render := func(frame string) {
				d.mu.Lock()
				d.screen.Write([]byte("\x1b[2J\x1b[H" + frame))
				d.mu.Unlock()
				select {
				case d.updated <- struct{}{}:
				default:
				}
			}
			render("| v default (1)              | preview |\r\n| > . restart claude starting | old pane text |")
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			result := make(chan error, 1)
			go func() { result <- sendNavKeySettled(ctx, d, "g") }()
			key := make([]byte, 1)
			if _, err := io.ReadFull(reader, key); err != nil {
				t.Fatal(err)
			}
			if string(key) != "g" {
				t.Fatalf("key=%q", key)
			}
			if previewOnly {
				render("| v default (1)              | preview |\r\n| > . restart claude starting | NEW pane text |")
				select {
				case err := <-result:
					t.Fatalf("preview-only repaint acknowledged g before selection moved; err=%v; selected=%q", err, selectedSidebarLine(d.Frame(false)))
				case <-time.After(80 * time.Millisecond):
				}
				render("| v default (1)              | preview |\r\n|   . restart claude starting | NEW pane text |")
			} else {
				render("| v default (1)              | preview |\r\n|   . restart claude starting | old pane text |")
			}
			select {
			case err := <-result:
				if err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal("real selection change was not acknowledged")
			}
		})
	}
}
```

Adapt the probe to the driver's current field names if they have drifted. The two subtests'
meaning is the obligation.

## R170 — a one-shot DB step cannot lose to Reconcile's self-heal

- `status_recovery.feature` has the scenario "r on a terminal row whose tmux session already
  exists never reaches an error, however fast Reconcile's self-heal lands". Its step `the state
  database session "dup pane" is "stopped" from "hook"` reads the store once. Reconcile's 250 ms
  self-heal (SPEC §7) can already have repaired the row to `starting` from `tmux` by then. Run
  36792029158 failed this way on the first attempt and on the solo rerun, before any `r` press.
- **After this phase**, the scenario observes the `stopped`-from-`hook` transition in a way that
  cannot lose that race:
  - by the durable record (the audited event or transition history);
  - or by holding Reconcile at a deterministic point the harness controls.

  The scenario still proves its requirement: `r` never reaches an error, however fast the
  self-heal lands. Audit `features/` for other one-shot store reads that a self-heal can
  overtake, and treat each one found the same way.
- **Success:**
  - A regression test forces the self-heal to land before the read. It fails on `a192accf7d`
    and passes after.
  - The scenario passes 20/20 solo runs under `-race`.

## R171 — `docs/ci.md` and comments stay true

- `docs/ci.md` says "only budget assertions consult racebuild.Enabled". Race-widened deadlines
  consult it too (Phase 4g residual R1). Correct that, and every passage or code comment that
  this phase makes false. One example is a comment that justifies the retry R169 removes.
- **Success:** no test pins `docs/ci.md`'s wording. Review judges this by reading, and wording
  is never a finding.

## Ordering

1. **The R168 inventory**, notified as soon as it is done.
2. **R169** and **R170**: known mechanisms with known reproductions.
3. **The R168 mechanisms**, the most frequent first. The transient-"starting"-frame class is the
   top one.
4. **R171.**
5. **The proof (§Definition of done), as the last task, with nothing committed after it.** If a
   proof run is red or flaky, that is a real defect:
   - add the failure to the inventory;
   - fix it at its root, with a regression test that fails before the fix;
   - push, and restart the proof from zero.

   That is the only reason to commit after the proof starts.

## Definition of done

- R168-R171's behaviours are in the live code, and each named test exists and is green. Each
  regression test fails on `a192accf7d`, with the evidence in artifacts.
- **At the final pushed sha:**
  - the `push` run of `ci` concluded `success`, and its flaky record is empty;
  - **five consecutive `workflow_dispatch` runs of `ci` concluded `success`, and each one's
    flaky record is empty.** That is the nightly path: `-race`, `ci/stability.sh 3`, report and
    publish. A scheduled nightly at the final sha counts as one of the five.
    - "Flaky record is empty" means `ci/suite.sh` reran no features scenario and no Go test, and
      the stability loop had no failure. Read it from the run's log and artifacts.
    - Dispatch them **one at a time**. Each one starts after the previous one has finished.
    - None runs while the job's own local `ci/stability.sh` or other heavy local work is
      running, so the proof measures CI and not this job's load on the shared host.
    - A red or flaky run resets the count to zero, with the exception in §Materiality rubric.
  - `ci/run.sh go test -p=1 -count=1 ./...` is green locally: the whole suite, with no narrowed
    package list and no `-run` filter;
  - a local `ci/stability.sh 5` is green with no failure in any run, and its logs are in
    artifacts. Run it **before** the dispatches, not alongside them;
  - `go build ./...`, `go vet ./...` and `gofmt -l` are clean on every file this run touched;
  - the protected-path audit command prints nothing.
- No secret is exposed.

Nothing else is required. In particular, no report, record or log row goes in the repo.

## Non-goals

- The quality gates in #54: coverage, CRAP, golangci-lint, govulncheck and Trivy.
- Allure BDD reporting (#55).
- Changing the release gate (`ci/releasegate`, `release.yml`), the workflows' triggers or
  concurrency, the runners, or the nightly's cron slot.
- Removing `ci/suite.sh`'s per-scenario rerun. It stays as a safety net and keeps recording
  flaky runs (SPEC §13.2). It must simply never fire during the proof.
- The CPU work (#49) and any feature work.
- Publishing a release.

## For the planner

- **Runner time is the scarce resource.** There are two shared `deck-ws-*` slots. A push run takes
  about 8 minutes. A dispatch run takes about 55 minutes. Five consecutive dispatches are about
  4.5 hours of wall clock.
  - Batch commits, and get local evidence (the 20/20 solo runs, `ci/stability.sh 5`) before
    spending a dispatch.
  - Poll the Actions API; do not sleep in a loop.
  - One iteration may dispatch a run and poll it to completion. Record each proof run's id,
    conclusion and flaky record in artifacts as it lands, so a later iteration can continue the
    count.
- **Reproduce before fixing.** Each mechanism's regression test comes first, and must fail on the
  unfixed tree. A fix with no failing-first test is not accepted.
- **`-race` multiplies latency** several times over. Many race-only first-attempt failures will be
  one mechanism (a transient, or an unsettled read) that `-race` merely exposes. Find the
  mechanism; do not scale every deadline.
- **Budget for one cure pass.** That is the harness working.
