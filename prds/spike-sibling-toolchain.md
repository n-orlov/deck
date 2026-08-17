# Spike: build and test `deck` in a sibling Go container

## Goal

Prove that a ralphd job can do **all** `deck` development work — compile, run unit tests,
drive real `tmux`, and drive a real TUI through a pty — inside a **sibling** container on
the host docker daemon, with the host workspace bind-mounted, and leave usable artifacts
behind on the host.

This is a throwaway spike whose only lasting deliverables are a reusable CI image
definition, a wrapper script, and a written report. If any requirement cannot be met,
**say so explicitly in the report with the exact error** — a documented failure is a
successful spike. Do not silently work around a blocked requirement.

## Background the agent needs

- This container has the docker CLI and the host docker socket. Containers you start are
  **siblings on the host daemon**, so every `-v` source path must be a **host** path.
- ralphctl injects the host paths: **`$RALPHD_HOST_WORKSPACE`** is the host path of the
  directory mounted here at `/workspace`. Use it for `-v`. Mounting `/workspace` as a
  sibling source would silently mount an empty directory.
- This job container runs as uid/gid `1000`, and so does the host user. Siblings must
  therefore run as `--user 1000:1000` so files they create in the workspace are not
  root-owned.
- Siblings run on docker's default bridge network and have normal internet access
  (image pulls, `go mod download`). They do **not** need the LLM gateway.
- Base image `golang:1.25-trixie` is already pulled on the host and provides Go 1.25.13;
  `tmux` is not in it and must be installed in the derived image.

## Requirements

1. **`ci/Dockerfile`** exists and builds an image tagged `deck-ci:local` from
   `golang:1.25-trixie`, adding at least `tmux`, `git` and `ca-certificates`. The build is
   performed by this job via `docker build`.
2. `deck-ci:local` reports **Go ≥ 1.25** and **tmux ≥ 3.2**, evidenced by the recorded
   output of `go version` and `tmux -V` run inside it.
3. **`ci/run.sh`** exists, is executable, and runs an arbitrary command inside a
   throwaway (`--rm`) `deck-ci:local` sibling with: `$RALPHD_HOST_WORKSPACE` mounted at
   `/w`, working directory `/w`, `--user 1000:1000`, and a **named docker volume**
   (`deck-go-cache`) mounted for `GOMODCACHE` and `GOCACHE` so caches survive between
   invocations. It exits with the wrapped command's exit code. Usage:
   `ci/run.sh go test ./...`.
4. A throwaway Go module exists under **`.spike/`** in the workspace (module path
   `example.com/spike`, any name) that compiles with `ci/run.sh go build ./...`.
5. **Unit test passes**: `.spike/` contains a trivial `go test` that passes when run via
   `ci/run.sh`, evidenced by recorded output.
6. **Real tmux works in the sibling**: a Go test in `.spike/` starts a tmux server on a
   **private socket** (`tmux -L <name>`), creates a detached session running a command
   whose output is predictable, reads it back with `capture-pane`, asserts the expected
   text, and kills the server. It passes via `ci/run.sh`, and leaves no tmux server
   behind.
7. **A pty-driven TUI works in the sibling**: a Go test in `.spike/` starts a **minimal
   bubbletea program** (a real `tea.Program`, not a stub) inside a pty, sends a keystroke,
   reads the rendered output, and asserts both the initial frame and the post-keystroke
   frame. It passes via `ci/run.sh`. This is the highest-risk requirement — if a pty or
   bubbletea cannot run in the sibling, the report must state exactly why.
8. **Host visibility**: a file written by a sibling inside the mounted workspace is
   visible from this job container under `/workspace`, and a file written here is visible
   to the sibling. Demonstrate both directions.
9. **No root-owned litter**: after all sibling runs, every file and directory created
   under the workspace is owned by uid `1000`. Evidence it with `find`/`stat` output. If
   anything is root-owned, that is a finding for the report, not something to hide.
10. **Cache works**: record the wall-clock duration of `ci/run.sh go test ./...` on a cold
    cache and on a warm cache (second run), and state both in the report.
11. **`ci/SPIKE.md`** is written and contains: the exact commands used, the recorded
    outputs proving requirements 2, 5, 6, 7, 8, 9 and the two timings from 10, the
    resolved versions (Go, tmux, bubbletea, docker), every gotcha discovered, and a clear
    verdict — is "all deck work in a sibling Go container" viable as the standing
    development approach, and what would the next phase need to be careful of.

## Constraints

- **Do not modify `SPEC.md`.** Do not modify anything under `prds/`.
- **Do not run `git commit`, `git push`, or any other git write command.** Leave all
  changes uncommitted in the working tree for the operator to review.
- Add `.spike/` to `.gitignore` (creating `.gitignore` if absent). `ci/` is a real
  deliverable and must **not** be ignored.
- Do not install anything into this job container; all toolchain work happens in siblings.
- Do not start long-lived containers. Every sibling is `--rm`. Clean up the
  `deck-ci:local` image only if you created intermediate junk tags; the final image should
  remain for reuse.
- Keep the spike small. This is not the start of the real implementation: do not create
  the `deck` module, do not implement anything from `SPEC.md`.
