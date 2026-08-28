# R78 evidence — an archived row keeps its name, and `dd` frees it

Requirement R78 (`SPEC.md` §9.2), tasks 007–009, fixing shas `47166c8`, `e699c31`,
`3247a7a`.

R78 is **not** one of the PRD's eight product-defect (naive-test-trap) requirements, so
no revert-and-reproduce red is owed for it; what is owed is a green run of the
assertions the report names. Tasks 007–009 left no per-task evidence directory, so these
two logs were **captured by task 038** at HEAD `e02ef08` (post-fix tree, unmodified) —
they are green-only confirmation runs, not implementation-time red/green pairs, and
nothing here should be read as one.

- `green-store-archived.log` —
  `ci/run.sh go test -count=1 -v -run 'TestDDOnAnArchivedRowIsReachableAndReapsCleanly|TestCreateSessionRefusesArchivedNameHolder|TestRenameSessionRefusesLiveAndArchivedHolders' ./internal/store/`
  (`internal/store/tombstone_test.go`: the archived-holder refusal wording for both
  create and rename, and the full `dd`-on-an-archived-row round trip).
- `green-filter-feature.log` —
  `ci/run.sh env DECK_GODOG_PATHS=filter.feature go test ./features/ -run TestFeatures -count=1`
  (`features/filter.feature`: the same round trip through real keystrokes).
