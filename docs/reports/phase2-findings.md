# Phase 2 design findings

This report is accumulated during Phase 2 for later operator review against `SPEC.md`.

## Frozen clock control

- `SPEC.md` §13.1 describes `DECK_CLOCK_STEP` as advancing the frozen clock “on demand” but does not define a collision-free control surface. The initially implemented `>` binding conflicts with the §11 sidebar-width keymap and was removed.
- The implemented contract is `$DECK_HOME/clock.now` when `DECK_HOME` is set, or `clock.now` under deck's resolved XDG data root otherwise. When `DECK_CLOCK` freezes wall time, a valid RFC3339/RFC3339Nano instant in that file overrides the initial value. Tests and external automation advance time by writing the desired absolute instant; all running and later deck processes sharing the data root then observe it.
- `DECK_CLOCK_STEP` remains the configured/suggested increment for test clock-control tooling; it does not claim a TUI key. The help overlay documents `clock.now` next to both clock environment controls.
