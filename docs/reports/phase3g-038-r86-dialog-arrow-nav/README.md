# R86 revert-and-reproduce, captured retroactively by task 038

**Deviation disclosed up front:** the standing rule asks every product-defect
requirement's red/green to be captured *at implementation time* by the task that
made the fix, with task 038 only assembling what already exists. Tasks 025/026
(the fixing shas, `a337671` + `8cff03b`) landed without a dedicated
`docs/reports/phase3g-02[56]-*/` evidence dir. Rather than let R86 go unproven in
the phase report, this directory was produced now, by task 038, through the
identical revert-and-reproduce method the other seven requirements used: the
fixing hunk was reverted in the live tree, the test run captured red, the hunk
restored (`git diff` empty afterwards, confirmed), and the same tests re-run
green. Nothing else in the tree was touched to produce it.

## What was reverted

`internal/tui/dialog_contract.go`'s two `case` labels in `applyDialogContract`,
swapped back to their pre-`a337671` names — the entire product-side diff task 025
made to this function:

```
-	case "down":
+	case "tab":
 		if c.Fields.Count <= 1 || c.Fields.Index == nil {
 			return nil, false
 		}
 		*c.Fields.Index = (*c.Fields.Index + 1) % c.Fields.Count
 		return nil, true
-	case "up":
+	case "shift+tab":
 		if c.Fields.Count <= 1 || c.Fields.Index == nil {
 			return nil, false
 		}
 		*c.Fields.Index = (*c.Fields.Index - 1 + c.Fields.Count) % c.Fields.Count
 		return nil, true
```

## Red (this revert applied)

`ci/run.sh go test -count=1 -v ./internal/tui/ -run 'TestApplyDialogContractCoreKeys|TestCreateModalUpDownMoveFieldsTabDoesNot|TestCreateModalTabOnPathFieldWithNoMatchDoesNotMoveFocus'`
— [`red-before-fix.log`](red-before-fix.log):

```
--- FAIL: TestApplyDialogContractCoreKeys/up_and_down_move_the_focused_field,_wrapping (0.00s)
    dialog_contract_test.go:55: down from last field: handled=false index=2, want wrap to 0
--- FAIL: TestApplyDialogContractCoreKeys/tab_and_shift+tab_are_not_contract_keys_(§11.4:_reserved_for_completion) (0.00s)
    dialog_contract_test.go:66: tab: handled=true index=1, want unhandled and unchanged
--- FAIL: TestCreateModalUpDownMoveFieldsTabDoesNot (0.00s)
    dialog_contract_test.go:137: "down" moved createField to 2, want 3
FAIL	github.com/n-orlov/deck/internal/tui	0.011s
```

## Green (revert undone, `git diff` confirmed empty)

`ci/run.sh go test -count=1 -v ./internal/tui/ -run 'TestApplyDialogContractCoreKeys|TestCreateModalUpDownMoveFieldsTabDoesNot|TestCreateModalTabOnPathFieldWithNoMatchDoesNotMoveFocus|TestCreateModalRecentCWDCyclesOnCtrlPCtrlN|TestCreateModalCandidateListOwnsUpDownWhileOpen'`
— [`green-after-fix.log`](green-after-fix.log): all PASS, `ok github.com/n-orlov/deck/internal/tui 0.037s`.

## The overlay non-regression and the on-screen-text/parity net, both already green

Not reverted (the requirement's own text: "assert it explicitly rather than
assuming `Count 0` protects it" — these are the assertions, not something to
break and fix again): `TestHelpOverlayKeymapMatchesBoundKeys`,
`TestFooterKeyLegendNamesOnlyBoundKeys` (the parity net task 026's commit
message says would catch a half-done change),
`TestScrollableOverlaysScrollOneLineWithArrows`,
`TestScrollableOverlaysScrollOneLineWithJK` (the `?`/`i`/`E` overlays still line-scroll
on `↑`/`↓` after the contract now binds those keys elsewhere) — all PASS,
[`overlays-and-parity-green.log`](overlays-and-parity-green.log).
