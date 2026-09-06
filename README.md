# deck

TUI session manager for CLI coding agents (Claude Code, pi, plain shell) on tmux.
Named sessions, durable conversations, resume on demand.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/n-orlov/deck/main/install.sh | sh
```

- Installs to `~/.local/bin/deck`. Override: `DECK_INSTALL_DIR`, `DECK_VERSION=vX.Y.Z`.
- Requires **tmux >= 3.2**. Agents (`claude`, `pi`) only appear if they are on `PATH`.
- Linux and macOS, amd64 and arm64. **No Windows** (needs tmux) -- use WSL.
- Check: `deck --version`. Update: rerun the same line.

## Run

```sh
deck
```

| key | action |
|-----|--------|
| `n` | new session (shell / claude / pi) |
| `↵` | interactive mode in the preview; `Ctrl+Q` leaves |
| `a` | full tmux attach |
| `r` / `x` / `u` | resume / kill / undo |
| `,` | settings |
| `?` | all keys |
| `q` | quit |

Config: `~/.config/deck/config.toml`. State: `~/.local/share/deck/`. tmux socket: `tmux -L deck`.

## Release

```sh
git tag v0.2.0 && git push origin v0.2.0
```

GitHub Actions ([release.yml](.github/workflows/release.yml)) builds the four binaries,
stamps the tag into `deck --version` and publishes the release with `checksums.txt`.

## Develop

```sh
go build ./cmd/deck                      # local build
ci/run.sh go test -p=1 -count=1 ./...    # full suite in the CI container (~6 min)
```

Behaviour is specified in [SPEC.md](SPEC.md). Phase records live in `docs/reports/`.
