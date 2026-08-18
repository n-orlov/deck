# Toolchain version evidence

Real output of resolving the delivered Go and tmux versions, captured by
running the exact commands through `ci/run.sh` (which starts a labeled,
`--rm` sibling container built from `ci/Dockerfile` on the host docker
daemon; see `ci/SPIKE.md`).

## Go version

Command:

```
ci/run.sh go version
```

Output:

```
go version go1.25.13 linux/amd64
```

Exit status: `0`

## tmux version

Command:

```
ci/run.sh tmux -V
```

Output:

```
tmux 3.5a
```

Exit status: `0`

## Notes

- Both commands were run against the same sibling container image
  (`deck-ci:local`, built from `ci/Dockerfile`) used for all other CI
  commands in this repository, so these are the exact Go and tmux versions
  the rest of the Phase 0 evidence in `docs/reports/` was produced against.
- No values were edited or hand-typed; this file reproduces the literal
  stdout of each command.
