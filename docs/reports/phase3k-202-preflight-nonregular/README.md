# Task 202 — pin the create preflight against a FIFO named like the agent binary

## What this covers

Task 201 hardened `lookPathIn` (`internal/service/availability.go`) to
require `info.Mode().IsRegular()` in both the path-separator branch and the
PATH-search loop, closing finding 1's FIFO gap. That task's own test
(`TestLookPathInRejectsFIFOEvenWithExecuteBits`,
`internal/service/availability_test.go`) proves the probe function itself
rejects a FIFO. This task pins the same regression one level up, at
`CreateAgent`'s own preflight (`internal/service/agent.go`), which calls
`lookPathIn` before any durable row or tmux pane exists (R111/task 010).

## New test

`TestCreateAgentPreflightRefusesFIFONamedLikeAgentBinary`
(`internal/service/agent_test.go`):

- creates a mode-0755 FIFO (`syscall.Mkfifo`) named `claude` in a directory
  placed on the service's `ConfigEnv["PATH"]` (the launch PATH `CreateAgent`
  resolves, per SPEC §6.3 — `ConfigEnv`'s own `PATH` entirely overrides
  `captured_path`),
- calls `CreateAgent` with `Agent: "claude"`, `LoginShell: false`,
- asserts the returned error's message contains `not found on PATH`,
- asserts the store holds no session row named `"Claude: fifo"` (the
  preflight must run before `Store.CreateSession`), mirroring the existing
  `TestCreateAgentPreflightRefusesAgentBinaryNotFoundOnPath` and
  `TestCreateAgentPreflightRefusesModeNonExecutableBinary` non-regression
  shape,
- asserts `service.TMux.List` reports zero live tmux sessions on the
  test's own private socket (the same absence check
  `internal/service/resume_test.go`'s tests already use), so the FIFO's
  refusal is proved before any pane is created, not merely before the
  first named row lands.

## Evidence

`ci/run.sh go test -count=1 -run 'TestCreateAgentPreflightRefusesFIFONamedLikeAgentBinary' -v ./internal/service/`
exits 0 — captured verbatim in `test.log`, exit code in `test.exitstatus`.

```
=== RUN   TestCreateAgentPreflightRefusesFIFONamedLikeAgentBinary
--- PASS: TestCreateAgentPreflightRefusesFIFONamedLikeAgentBinary (0.03s)
PASS
ok  	github.com/n-orlov/deck/internal/service	0.027s
```

Also re-ran the package's measured targeted surface as a sanity check
(not re-captured as a separate artifact, per the standing rule against
re-measuring): `ci/run.sh go test -count=1 ./internal/service/ ./internal/agent/`
exits 0.

## Scope note

No product code changed in this task — task 201 already did the fix
(`lookPathIn`'s `IsRegular()` check). This task only adds the
`CreateAgent`-level pin the plan's task 202 calls for.
