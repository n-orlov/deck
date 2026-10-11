# Phase 4p — trailing-slash cwd resume (#79) and escaped-JSON redaction (#80)

## What the operator gets

1. Resuming a Claude session whose stored cwd has a trailing slash (or any other non-canonical form)
   resumes it instead of failing with "Session ID … is already in use" (#79). The transcript lookup
   (`claudeTranscriptLookup` in `internal/agent/claude.go`) cleans the cwd before encoding it, a session
   created with a non-canonical cwd stores the cleaned one, and the sibling agents' lookups are checked
   for the same defect.
2. The event hook's redaction masks secrets inside JSON that was serialised into a JSON string, such as
   `{\"api_key\":\"sk_live_abcdef123\"}` and `{\"password\": \"hunter2hunter2\"}` (#80). The
   unescaped forms stay masked, and nothing already masked becomes unmasked.

## Issues in scope

#79 and #80, nothing else. **Not in scope: #73** and the LOW observations in #80
(`user:pass@host` URLs, `Authorization: Basic`, URL-encoded separators).

Code to follow: `internal/agent/claude.go` (`claudeTranscriptLookup`, `Launch`, `Resume`), the sibling
agent lookups under `internal/agent/`, the session-creation path in `internal/service/`,
`internal/notify/redact.go` (`quotedKeyValueAt`, `findKeyValuePair`), `SPEC.md` §8 and §10.
Symbols are as of `8b2181621d4`; drift is expected and is not a finding.

## Requirements

- **R244 (#79).** A table test with one case per finite shape (`/a/b`, `/a/b/`, `/a/b//`, `/a/./b`,
  `/a/b/../b`) shows the transcript lookup finds the same transcript for all five; it fails at the
  parent for the trailing-slash shapes. A test shows `Resume` is chosen, not `Launch`, when the transcript
  exists under the cleaned path. Session creation stores the cleaned cwd; a test pins that.
- **R245 (#79).** Each sibling agent that derives a path from the cwd (read the code to enumerate them)
  either already cleans it, shown by a test, or is fixed with one. A pre-existing row with a trailing slash
  is read through the cleaned form and needs no migration.
- **R246 (#80).** A table test in `internal/notify/` has one case per finite shape: escaped key with
  `":"` and `": "` separators, an escaped key with a space before the colon, and two levels of escaping
  (`\\\"`). Each case fails at the parent. Existing cases in the shape table pass unchanged.
- **R247 (#80).** A test drives one escaped shape through a capture script and shows it masked in
  `DECK_EVENT_MESSAGE`, in the stdin JSON and in the stored script-output tail. A 1 MiB input of repeated
  backslashes and one of repeated `\"k\":` each finish in well under a second, so no quadratic scan
  is introduced. SPEC.md §10 lists the escaped shape.

## Standing rules

- **Test recipes, verbatim:**
  - one package: `ci/run.sh go test -count=1 ./internal/<pkg>/`
  - one scenario file: `DECK_GODOG_PATHS=<file>.feature ci/run.sh go test -count=1 ./features/ -run TestFeatures`
  - lint: `ci/run.sh ci/lint.sh`; the gates: `ci/run.sh ci/quality.sh`
  - the whole suite: `ci/run.sh ci/suite.sh`, never run in parallel with another heavy command.
  - Always run from a shell with no `DECK_*` variable set.
- **Only the final sweep runs a whole tier.** Every other task runs the narrowest recipe that covers what
  it touched, plus `ci/run.sh ci/lint.sh` before every push.
- **Commits are made per task and pushed to `main`.** Title `<area>: <imperative summary> (task NNN, R###)`.
- **After every push**, at the start of the next iteration and before picking a task, poll that push run
  to its conclusion via the Actions API. A red run is the next task, fixed at its root.
- **Waits.** A wait longer than about 85% of the iteration cap is spanned with `deferred`, never by
  sleeping past the cap.
- **The sweep's shape** (the last three tasks, in this order, all at the same `origin/main` head):
  1. the whole `go test -p=1 -count=1 ./...` tier under `ci/run.sh`, then `go build ./...`,
     `go vet ./...` and `gofmt -l` on the tracked `.go` files;
  2. `ci/run.sh ci/suite.sh ci-results && ci/run.sh ci/quality.sh ci-results`;
  3. confirm the push run at that head is `success`, then dispatch `ci.yml` **once** and read its flaky
     record.
  Sweep tasks commit nothing. **Nothing is committed after the sweep.** If `origin/main` moved past the
  swept sha, the sweep is stale and re-runs. A stale sweep is never process.
- **The CI dispatch recipe.** Use the Actions REST API with the operator's PAT from the git credentials:
  `POST /repos/n-orlov/deck/actions/workflows/ci.yml/dispatches` with `{"ref":"main"}`, then poll
  `GET /repos/n-orlov/deck/actions/runs?event=workflow_dispatch&head_sha=<sha>` to its conclusion, and
  read the run's log and artifacts. Dispatch **once**, at the final pushed sha. A run that never reached
  `suite` (runner provisioning failure, API 5xx) is not judged: dispatch another.
- **Engine defects to avoid by hand.** The engine cannot be patched by this run:
  - a task titled "every X" over an open domain becomes a stall-detector trap, so this PRD names its
    finite cases and so must the tasks;
  - do not signal COMPLETE while tasks are pending;
  - when an approach ends in review, read the findings before replanning;
  - after any cure pass, check `tasks.json` for a task that depends on itself and repair it by steer.


## Non-negotiables

- **No weakened test.** A test pinning behaviour this phase changes is rewritten in the same commit,
  naming the covering test it becomes. Nothing else about the suite is retired.
- **No raised timeout, budget, worker count or per-test bound.** `internal/racebuild` is untouched.
- **No new required contract field** in a hook or event payload deck *receives* from an agent.
- **No gate loosened and no exception added** (`ci/quality.json` only ever tightens).
- **Existing agents behave identically** except where a requirement names the change.
- **Profiles stay isolated.** No code path reads or writes another profile's config, state or socket.
- **A config reload never writes.**
- **The event hook never blocks the agent or loses an event**; redaction only ever removes text.

