# R92 evidence — R75's release-failure fallback is exercised through a seam

Requirement R92 (finding F17), tasks 036–037, fixing shas `78bc156` (the
`LaunchLeaseReleaser` seam) and `cc36cfa` (the test that drives it).

R92 is explicitly *not* a behaviour change — it proves a pre-existing degradation is
graceful — so no revert-and-reproduce red is owed or claimed (see the report's R92
section). This green log was **captured by task 038** at HEAD `e02ef08` (unmodified
tree).

- `green-lease-release-failure.log` — `ci/run.sh go test -count=1 -v -run
  'TestResumeAuditsAndStaysTTLBoundedWhenReleasingTheLaunchLeaseFails'
  ./internal/service/` (`internal/service/lease_release_failure_test.go`: the audit
  event fires, the lease columns stay set, the caller's verdict is unchanged, and a
  later resume past the TTL succeeds).
