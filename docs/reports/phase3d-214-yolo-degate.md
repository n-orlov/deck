# Steer 017 item 2 — remove the yolo confirm, add `yolo_default` (task 214)

## What changed

SPEC.md §5/§6.5 (post-`395babf`) says `yolo` needs "no further ceremony" once
`allow_yolo = true`: choosing it at create time or via `P` "takes effect directly", and
`yolo_default` (default false, inert unless `allow_yolo` is also true) opens the create
modal already on `yolo`. Before this task, `internal/tui`'s create modal and the `P`
profile-switch dialog both required an additional explicit `y` confirm keystroke once
`yolo` was selected — a double-gate SPEC no longer describes.

This task:

1. Removed the `y`-confirm double-gate entirely (`createYoloConfirmed`,
   `profileSwitchYoloOK`, both fields and every branch that read/set them) from
   `internal/tui/tui.go`. `allow_yolo` alone still gates whether `yolo` is offered at all
   (`createProfileOptionsFor`); nothing about that gate changed.
2. Added a new flat config key `yolo_default` (default `false`) to `internal/config.Schema`
   (`internal/config/schema.go`), wired through `FileConfig`/`Settings`
   (`internal/config/toml.go`, `toml_write.go`, `config.go`) and the settings takeover
   (`internal/tui/settings.go`) — it appears in the settings view for free, via the same R7
   schema mechanism `allow_yolo` already used, not a hand-rolled row.
3. `yolo_default=true` with `allow_yolo=false` is a stated, visible inconsistency in the
   `yolo_default` row's own text ("(inert: allow_yolo is off, so this has no effect yet)"),
   never a silent override of either key in either direction
   (`settingsFieldValueDisplay` in `internal/tui/settings.go`).
4. The create modal opens already on `yolo` when both `yolo_default` and `allow_yolo` are
   true, for an adapter that actually declares `yolo` support (`Model.defaultCreateProfile`,
   `internal/tui/tui.go`). Non-obvious bug found and fixed while proving this: the very
   first agent the modal defaults to (`defaultCreateAgent`) is always `"shell"`, which has no
   permission-profile notion at all and never offers `yolo` — so `defaultCreateProfile` at
   modal-open time alone is not enough to observe `yolo_default` unless the user is already
   on an agent that offers `yolo`. Fixed by re-running `defaultCreateProfile` for the new
   agent whenever the Agent field is cycled (`cycleCreateField` case 2), UNLESS the user has
   already touched the Permission profile field themselves (new `createProfileTouched`
   bool) — a deliberate choice is never silently overridden by a later agent change, but the
   *default* tracks the currently-selected agent's own default until the user picks
   something themselves. Proved by the new
   `yolo_default_opens_the_create_modal_already_on_yolo_once_allow_yolo_is_enabled` scenario,
   which opens the modal (default agent `shell`, profile `safe`) then cycles the Agent field
   to `claude` and asserts the profile row already reads `yolo`.
5. Help text updated in the same commit as the key removal (guard-word convention):
   `cmd/deck/main_test.go`'s `"Yolo is gated twice"` assertion → `"Yolo is gated by
   allow_yolo"` plus a new `"yolo_default"` mention; `internal/tui/tui.go`'s help paragraph
   rewritten to describe the single gate and `yolo_default`'s effect and inertness; the
   `dialog_contract.go` comment that named the `y` confirm as a dialog's additional
   load-bearing key updated to say it used to be one.
6. `features/permission_modes.feature`: the confirm scenario is REPLACED by its inverse
   ("yolo takes effect immediately with no confirm once allow_yolo is enabled") — this is a
   requirement change, not a deleted assertion, stated as such in both the scenario's own
   comment and the commit message. A new scenario
   ("yolo_default opens the create modal already on yolo once allow_yolo is enabled") is
   added. The `allow_yolo`-gates-`yolo` scenario ("yolo is unavailable without allow_yolo
   enabled") is unchanged and still passes. The `confirming yolo` step phrase is retired
   (`clientCreatesAgentSessionConfirmingYolo`/`clientAttemptsAgentSessionWithYoloWithoutConfirming`
   removed from `features/agent_steps_test.go`) since every remaining scenario now just
   creates a yolo session directly via the existing `with permission profile "yolo"` step.
7. Two `features/settings.feature` scenarios that counted `j` keypresses through the General
   category's field list to land on `Stale After` needed one extra `j` each, because
   `yolo_default` is now a field between `allow_yolo` and `stale_after` in that list. Fixed
   with a comment at each site explaining why. No other `.feature` scenario's navigation was
   affected (checked every scenario that Tab/`j`s into the General category's fields; every
   other affected-looking scenario navigates a different category or asserts nothing
   position-dependent).

## Non-vacuousness / evidence

- `internal/tui` unit tests: `TestCreateModalYoloTakesEffectImmediatelyWithNoConfirm`,
  `TestCreateModalYCharacterStillTypesIntoTextFields`,
  `TestCreateModalDefaultsToSafeWhenYoloDefaultFalse`,
  `TestCreateModalDefaultsToYoloWhenYoloDefaultAndAllowYoloAreBothTrue`,
  `TestCreateModalYoloDefaultIsInertWithoutAllowYolo`,
  `TestCreateModalYoloDefaultSkippedForAnAdapterThatDoesNotOfferYolo`,
  `TestProfileSwitchToYoloTakesEffectWithNoConfirm` — all green
  (`ci/run.sh go test -count=1 ./internal/tui/... ./internal/config/...`: both packages `ok`).
- `internal/config` schema tests (`TestSchemaPinsKeySet`, `TestSchemaScopes`,
  `TestSchemaFieldsAreComplete`) extended for `yolo_default` and green.
- Isolated feature run, `permission_modes.feature` + `settings.feature` together (23
  scenarios, via a scratch `Paths` override reverted before commit — `git diff` on
  `features/godog_test.go` is empty): all 23 `PASS`, 0 `FAIL`
  (`ci/run.sh sh -c 'go test -count=1 -run TestFeatures ./features/ -v'`, 20.98s).
- `ci/run.sh go build ./...`, `go vet ./...`, `gofmt -l $(git ls-files '*.go')`: all clean.

## SPEC.md gap check

No delta needed — see `docs/reports/phase3d-spec-deltas.md`'s item 2 section. SPEC.md §5's
own text was matched exactly (quoted there); code and docs agree.
