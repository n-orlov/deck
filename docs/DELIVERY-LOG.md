# deck — delivery log

Append-only record of phases actually run. `docs/PLAN.md` is the intended order;
`SPEC.md` is the product spec.

Each phase is one PRD run as one autonomous job. The **PRD blob** column is the git hash of
the exact PRD file contents used for that run — PRDs get edited between phases, so the path
alone is not an identification. Recover any past version with `git cat-file -p <blob>`.

## Phases

| # | Phase | PRD | PRD blob | Run ID | Started | Completed | Engine verdict | Verified |
|---|---|---|---|---|---|---|---|---|
| — | Toolchain spike | `prds/spike-sibling-toolchain.md` | `7ae5e05` | `deck-spike-sibling` | 2026-08-17 14:41 | 2026-08-17 14:54 | `failed / unverified` — iteration budget (10) exhausted at the audit task | **pass**, operator-verified: 4/4 tests re-run independently, ownership clean, both mount directions proven |
| 0 | Harness & walking skeleton | `prds/phase0-harness-and-skeleton.md` | `40af336` | `deck-phase0` | 2026-08-17 18:09 | *in progress* | — | — |

### Notes per run

**Toolchain spike** — proved all deck work can run in a sibling Go+tmux container with the
host workspace bind-mounted. Deliverables kept: `ci/Dockerfile`, `ci/run.sh`, `ci/SPIKE.md`.
Measured: Go 1.25.13, tmux 3.5a, cold suite 4.9 s / warm 1.1 s, no root-owned files.
The engine verdict was `unverified` purely because the 10-iteration budget ran out one task
short of its own audit step; every requirement had landed and was verified by hand instead.

Two defects were fixed in the delivered `ci/run.sh` afterwards: the cache volume had been
labelled with the run id *and hard-failed when it didn't match*, which would have broken
every subsequent job, and the script required `RALPHD_*` env so it only worked inside a job
container. It now shares the cache across runs and also works on the host.

Its most valuable output was a gotcha, now recorded in `SPEC.md` §13.2: **a pty is not a
terminal emulator** — bubbletea probes for background colour (OSC 11) and cursor position
(CPR) at start-up and blocks on the replies before rendering frame one, so a harness that
only reads will hang and look like a broken TUI.

**Phase 0** — running with `--model-strategy balanced`, strong `gpt-5.6-sol` (planning,
review, reflect) and fast `gpt-5.6-terra` (worker, verify), `--vigilant`, 40 iterations,
4 h cap, `--allow-docker --network host`. Under 30-minute operator oversight.

## Other milestones

| Date | What |
|---|---|
| 2026-08-17 | Repo created and published as `n-orlov/deck`; initial commit is the product spec |
| 2026-08-17 | `SPEC.md` v2: TUI-only, four agents, no daemon, pluggable notifications, BDD/black-box testability as a requirement |
| 2026-08-17 | Spec reviewed adversarially by a second model. Four of its "factual" findings were rejected against verified CLI/docs evidence; the rest were applied — debounce dropped (it required a daemon that the design forbids), `remain-on-exit failed` adopted so crash tails are capturable at all, Codex id discovery made serialised and claim-based, the three conflicting state machines reconciled into one, and dedupe given an epoch so a recurring prompt can't be muted forever |
| 2026-08-17 | "Toolchain in a sibling" upstreamed into ralphd itself (`n-orlov/ralphd` `a5a18d2`) as prompt-level guidance, docs, a mountable skill and 6 tests, so any future job gets the capability without a PRD explaining it |
