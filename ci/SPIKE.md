# Sibling-container development spike

## Verdict

**Viable.** A labeled, throwaway `deck-ci:local` sibling can compile and test Go,
start and clean up a private tmux server, and drive a real Bubble Tea program through
a pty while using the host workspace and a persistent named Go cache. The next phase
should keep all container launches behind `ci/run.sh`, run as UID/GID 1000, use host
paths for bind sources, give each tmux test a unique private socket, and emulate terminal
capability replies in bare-pty tests.

## Image and wrapper

The reusable image was built from the repository root with:

```sh
docker build --label ralphd.run=deck-spike-sibling -t deck-ci:local -f ci/Dockerfile .
```

`ci/run.sh` uses `docker run --rm`, applies
`--label ralphd.run=$RALPHD_RUN_ID`, runs as `--user 1000:1000`, mounts
`$RALPHD_HOST_WORKSPACE` at `/w`, uses `/w` as its working directory, and mounts the
labeled `deck-go-cache` volume at `/go-cache`. The image sets `GOMODCACHE` and
`GOCACHE` to directories below that mount. All commands below that use `ci/run.sh`
therefore ran in labeled `--rm` siblings.

Resolved versions (unedited output):

```console
$ docker version --format 'Client {{.Client.Version}}; Server {{.Server.Version}}'
Client 29.7.2; Server 29.5.3
$ ci/run.sh sh -c 'go version; tmux -V; cd .spike && go list -m github.com/charmbracelet/bubbletea'
go version go1.25.13 linux/amd64
tmux 3.5a
github.com/charmbracelet/bubbletea v1.3.10
```

The final image and volume carry the run label:

```console
$ docker image inspect --format '{{index .Config.Labels "ralphd.run"}}' deck-ci:local
deck-spike-sibling
$ docker volume inspect --format '{{.Name}} label={{index .Labels "ralphd.run"}}' deck-go-cache
deck-go-cache label=deck-spike-sibling
```

## Test evidence

The baseline unit test:

```console
$ ci/run.sh sh -c 'cd .spike && go test -v -count=1 -run ^TestGreeting$ ./...'
=== RUN   TestGreeting
--- PASS: TestGreeting (0.00s)
PASS
ok  	example.com/spike	0.003s
```

The tmux test creates a unique `tmux -L deck-spike-<pid>-<nanoseconds>` server,
starts a detached session that prints `deck-tmux-ready`, checks `capture-pane`, kills
the server, and verifies that the same private socket no longer answers:

```console
$ ci/run.sh sh -c 'cd .spike && go test -v -count=1 -run ^TestTmuxPrivateServer$ ./...'
=== RUN   TestTmuxPrivateServer
    tmux_test.go:62: captured deterministic text and confirmed private server "deck-spike-79-1786978375515078255" is absent
--- PASS: TestTmuxPrivateServer (0.03s)
PASS
ok  	example.com/spike	0.037s
```

A post-suite socket probe also produced no output, so no private test server remained:

```console
$ ci/run.sh sh -c "find /tmp/tmux-1000 -maxdepth 1 -type s -name 'deck-spike-*' -print 2>/dev/null || true"
```

The pty test starts a real `tea.Program`, observes its initial frame, writes `x`,
observes the distinct changed frame, writes `q`, and checks clean exit:

```console
$ ci/run.sh sh -c 'cd .spike && go test -v -count=1 -run ^TestBubbleTeaThroughPTY$ ./...'
=== RUN   TestBubbleTeaThroughPTY
    pty_test.go:160: observed distinct initial and post-keystroke frames from a real Bubble Tea program
--- PASS: TestBubbleTeaThroughPTY (0.04s)
PASS
ok  	example.com/spike	0.042s
```

After these runs, no stopped image-based sibling existed; the empty output is expected
because `--rm` removed every sibling:

```console
$ docker ps -a --filter ancestor=deck-ci:local --format '{{.ID}} {{.Status}} {{.Names}}'
```

## Bind-mount visibility and ownership

A job-written file was read from the sibling, then a sibling-written file was read at
`/workspace` by this job container:

```console
$ printf 'job-to-sibling-visible\n' > .spike/job-written.txt
$ ci/run.sh cat .spike/job-written.txt
job-to-sibling-visible
$ ci/run.sh sh -c 'printf "sibling-to-job-visible\n" > .spike/sibling-written.txt'
$ cat .spike/sibling-written.txt
sibling-to-job-visible
$ stat -c '%u:%g %n' .spike/job-written.txt .spike/sibling-written.txt
1000:1000 .spike/job-written.txt
1000:1000 .spike/sibling-written.txt
```

The complete workspace ownership audit printed nothing: there were no entries on this
filesystem whose owner was not UID 1000.

```console
$ find . -xdev ! -uid 1000 -printf '%u:%g %p\n'
```

## Persistent cache and timings

For a genuinely cold measurement, the old cache was removed and recreated with the
required label. The first full suite downloaded modules; the second reused the same
volume. These are wall-clock timings from Bash's `time` keyword:

```console
$ docker volume rm deck-go-cache && docker volume create --label ralphd.run=deck-spike-sibling deck-go-cache
deck-go-cache
deck-go-cache
$ TIMEFORMAT='wall=%R seconds'; time ci/run.sh sh -c 'cd .spike && go test -count=1 ./...' # cold
go: downloading github.com/creack/pty v1.1.24
go: downloading github.com/charmbracelet/bubbletea v1.3.10
go: downloading github.com/charmbracelet/lipgloss v1.1.0
go: downloading github.com/charmbracelet/x/ansi v0.10.1
go: downloading github.com/muesli/ansi v0.0.0-20230316100256-276c6243b2f6
go: downloading github.com/muesli/cancelreader v0.2.2
go: downloading github.com/charmbracelet/x/term v0.2.1
go: downloading github.com/rivo/uniseg v0.4.7
go: downloading github.com/charmbracelet/x/cellbuf v0.0.13-0.20250311204145-2c3ea96c31dd
go: downloading github.com/muesli/termenv v0.16.0
go: downloading github.com/mattn/go-runewidth v0.0.16
go: downloading golang.org/x/sys v0.36.0
go: downloading github.com/lucasb-eyer/go-colorful v1.2.0
go: downloading github.com/charmbracelet/colorprofile v0.2.3-0.20250311203215-f60798e515dc
go: downloading github.com/aymanbagabas/go-osc52/v2 v2.0.1
go: downloading github.com/mattn/go-isatty v0.0.20
go: downloading github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e
ok  	example.com/spike	0.056s
wall=4.850 seconds
$ TIMEFORMAT='wall=%R seconds'; time ci/run.sh sh -c 'cd .spike && go test -count=1 ./...' # warm
ok  	example.com/spike	0.070s
wall=1.094 seconds
```

A sentinel independently proves that the named mount survives separate wrapper
invocations:

```console
$ ci/run.sh sh -c 'printf "cache-survives\n" > /go-cache/spike-sentinel'
$ ci/run.sh cat /go-cache/spike-sentinel
cache-survives
```

## Gotchas and cautions

- Sibling bind sources are host-daemon paths. `$RALPHD_HOST_WORKSPACE` must be used;
  mounting this job container's `/workspace` would expose an unrelated empty host path.
  Docker build context, in contrast, is sent by this job's Docker CLI, so `.` or
  `/workspace` works while the host-only workspace path is not visible in this
  container's filesystem namespace.
- The repository root intentionally has no Go module; the throwaway module is
  `/w/.spike`. Thus module commands need `sh -c 'cd .spike && ...'`. The PRD shorthand
  `ci/run.sh go test ./...` from `/w` would fail outside a module.
- Do not use `sh -lc` here: the login shell resets `PATH` and produced the exact error
  `go: not found`. `sh -c` preserves the image's Go path.
- Bubble Tea v1.3.10 probes background color and cursor position during package
  initialization. A pty transports bytes but is not a terminal emulator, so the test
  answers OSC 11 and CPR probes before asserting frames. Without those replies,
  initialization waits rather than rendering the first frame.
- `/usr/bin/time` is not installed in the job container (exact error:
  `/bin/bash: line 6: /usr/bin/time: No such file or directory`), so timings use Bash's
  built-in `time` and `TIMEFORMAT`; no package was installed in the job container.
- The cache volume is deliberately owned for UID 1000 by the image and every sibling
  runs as 1000:1000. Keep that invariant to avoid root-owned workspace or cache files.
