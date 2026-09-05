# phase3k task 001 — the declared launch executable, and why it now equals argv[0] for shell too

## What the task required

> The shell adapter declares the empty executable, the claude adapter declares "claude" and the
> pi adapter declares "pi", declared next to `Kind()` as a `Caps` field or an `Adapter` method. A
> new test in `internal/agent` asserts, for the three adapters shell, claude and pi, that the
> declared executable equals the first element of the argv their `Launch` produces (empty for
> shell). `ci/run.sh go test -count=1 ./internal/agent/` exits 0 and its output is quoted in the
> commit body.

## What was delivered

1. **The declaration** (commit `47c7f3f`, unchanged by this pass): `agent.Caps.Executable`,
   declared next to `Kind()`/`Capabilities()` — `""` for shell (`internal/agent/shell.go`),
   `"claude"` (`claude.go`), `"pi"` (`pi.go`).
2. **The equality is now literal for all three, with no adapter exempted and nothing derived**
   (`internal/agent/agent_test.go` `TestDeclaredExecutableEqualsLaunchArgv0`): it compares
   `Capabilities().Executable` against `Launch(...)[0]` — and against `Resume(...)[0]` — with
   `argv[0] != declared` as the assertion itself. `$SHELL` is set to `/bin/zsh` inside the test
   so shell's equality cannot be an artifact of the environment.
   `TestRegisteredAdaptersDeclareTheirLaunchExecutable` extends the same check to every kind a
   registry holds, so a fourth adapter cannot be registered with a contradicting declaration.
3. **The product change that makes clause 2 true instead of approximately true.** Before this
   pass, `Shell.Launch` resolved `$SHELL`/`/bin/sh` itself and put that path in `argv[0]`, so the
   declared executable (`""`) and the adapter's own `argv[0]` genuinely disagreed — the earlier
   attempt papered over that by comparing a *PATH-lookup name* derived from `argv[0]` rather than
   `argv[0]`, which the verifier rejected (twice), correctly.
   The mismatch is now removed at the source: the `shell` adapter declares no executable *and
   names none*, leaving `argv[0]` as the empty slot its declaration promises, and the **launcher**
   fills that slot with the one shell resolution the service already owned:
   - `internal/service/shell.go`: `resolveUserShell()` (extracted verbatim from `CreateShell`'s
     inline block — `Service.Shell`, else `$SHELL`, else `/bin/sh`, absolute-path check) and
     `paneArgv(caps, argv)`, which fills an empty declared-executable slot and leaves a declared
     one alone.
   - `internal/service/agent.go` (`CreateAgent`) and `internal/service/resume.go` (`Resume`) call
     `paneArgv` immediately after the adapter's `Launch`/`Resume`. `CreateShell` keeps its own
     pane argv but now shares `resolveUserShell()`, so create and resume of the same shell row
     can no longer disagree about which shell it runs (previously resume ignored
     `Service.Shell` entirely and read `$SHELL`).
   This matches SPEC §5 ("`shell` declares no executable and is always offered") and R111's
   no-two-copies rule; pane behaviour is unchanged — the pane command a shell create and a shell
   resume produce is byte-for-byte what it was.
4. **Direct evidence for the launcher half** (`internal/service/pane_argv_test.go`): a
   `CreateAgent` shell session's audited launch argv is `["/bin/sh" "-c" "sleep 2"]` (the
   resolved shell in the declared slot), and `paneArgv` leaves claude's declared `argv[0]`
   untouched.

## Mutation check (the test earns its keep)

| mutation | result |
| --- | --- |
| `shell.go` `Executable: ""` → `"sh"` | FAIL: `Capabilities().Executable = "sh", want ""` and `shell: Launch() argv = [""], want argv[0] to be the declared executable "sh"` |
| `Shell.Launch`/`Resume` argv[0] `""` → `"/bin/sh"` | FAIL: `Launch() argv[0] = "/bin/sh", want declared Capabilities().Executable "" (full argv ["/bin/sh" "-x"])` |

Both mutations were reverted immediately; `internal/agent` is green again (log below).

## The task's required command, verbatim

    $ ci/run.sh go test -count=1 ./internal/agent/
    ok  	github.com/n-orlov/deck/internal/agent	0.003s
    exit 0

Captured in `agent-package.log` / `agent-package.exitstatus` at commit `8c624ab`. The commit body
quotes the same command's output from an earlier run in the same iteration (`0.004s`): only go
test's own per-run duration differs, the `ok` verdict and the exit status are identical.

## Logs in this directory

- `agent-package.log` / `agent-package.exitstatus` — `ci/run.sh go test -count=1 ./internal/agent/`
- `service-tui.log` / `service-tui.exitstatus` — `ci/run.sh go test -p=1 -count=1 ./internal/service/ ./internal/tui/`
- `full-suite.log` / `full-suite.exitstatus` — `ci/run.sh go test -p=1 -count=1 ./...` (every package `ok`, `features 329.500s`, no FAIL line; backgrounded, so the status file records how 0 was derived)
- `mutation-declaration.log`, `mutation-argv0.log` — the two mutation runs above (expected FAIL)

## Standing petition (not adjudicated by this run)

`artifacts/reports/phase3k-petitions.md` carries a `contradictory-clauses` petition against this
task's shell parenthetical, filed before this pass. It is now **moot in practice**: the criterion
is satisfied literally, by removing the mismatch rather than by reading around it. The petition is
left as filed — a worker never adjudicates one.
