<!-- badges:start -->
[![CI](https://github.com/n-orlov/deck/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/n-orlov/deck/actions/workflows/ci.yml?query=branch%3Amain)
[![lint](https://img.shields.io/endpoint?url=https%3A%2F%2Fn-orlov.github.io%2Fdeck%2Fbadges%2Flint.json)](https://github.com/n-orlov/deck/actions/workflows/ci.yml?query=branch%3Amain)
[![unit tests](https://img.shields.io/endpoint?url=https%3A%2F%2Fn-orlov.github.io%2Fdeck%2Fbadges%2Funit-tests.json)](https://github.com/n-orlov/deck/actions/workflows/ci.yml?query=branch%3Amain)
[![features](https://img.shields.io/endpoint?url=https%3A%2F%2Fn-orlov.github.io%2Fdeck%2Fbadges%2Ffeatures.json)](https://github.com/n-orlov/deck/actions/workflows/ci.yml?query=branch%3Amain)
[![coverage](https://img.shields.io/endpoint?url=https%3A%2F%2Fn-orlov.github.io%2Fdeck%2Fbadges%2Fcoverage.json)](https://n-orlov.github.io/deck/)
[![vulnerabilities](https://img.shields.io/endpoint?url=https%3A%2F%2Fn-orlov.github.io%2Fdeck%2Fbadges%2Fvulnerabilities.json)](https://github.com/n-orlov/deck/actions/workflows/ci.yml?query=branch%3Amain)
[![license](https://img.shields.io/badge/license-Apache--2.0-blue)](https://github.com/n-orlov/deck/actions/workflows/ci.yml?query=branch%3Amain)
[![Go version](https://img.shields.io/badge/go-1.25.0-00ADD8)](https://github.com/n-orlov/deck/actions/workflows/ci.yml?query=branch%3Amain)
[![report](https://img.shields.io/badge/report-Allure-blue)](https://n-orlov.github.io/deck/)
<!-- badges:end -->

# deck

TUI session manager for CLI coding agents (Claude Code, pi, Codex CLI, Copilot CLI, plain shell) on tmux.
Named sessions, durable conversations, resume on demand.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/n-orlov/deck/main/install.sh | sh
```

- Installs to `~/.local/bin/deck`. Override: `DECK_INSTALL_DIR`, `DECK_VERSION=vX.Y.Z`.
- Verifies the download against `checksums.txt`, then, when an authenticated `gh` is installed, its
  build attestation (`gh attestation verify --repo n-orlov/deck`); a failed verification aborts with
  nothing installed. Without `gh` (or for a release that predates attestations) it prints one note
  that only the checksum was verified; `DECK_REQUIRE_ATTESTATION=1` makes that case abort instead.
- Requires **tmux >= 3.2**. Agents (`claude`, `pi`, `codex`, `copilot`) only appear if they are on `PATH`.
- Linux and macOS, amd64 and arm64. **No Windows** (needs tmux) -- use WSL.
- Check: `deck --version`. Update: `deck update` (or rerun the same line). `deck update --check` only reports whether a newer release exists, and `deck update --version vX.Y.Z` pins a release (a downgrade is allowed). It verifies the archive's checksum and, with an authenticated `gh`, its build attestation; without one only the checksum is checked (it catches corruption, not a tampered release), and `DECK_REQUIRE_ATTESTATION=1` makes that case abort. The `ctrl+u` confirmation in deck says which of the two will happen before it runs, and the footer's `update installed` note says `checksum-only` when only the checksum was verified. Before downloading, it also asks the GitHub release API whether the release is an immutable release and says so; one that is not verified immutable (every release up to v0.2.18, and any release from a `DECK_UPDATE_URL` mirror) still installs with a note, unless `DECK_REQUIRE_IMMUTABLE=1`, which refuses it. It refuses a dev build, a package-managed install (Homebrew, apt, Nix, snap) and an unwritable directory. Restart a running deck afterwards. A profile named `update` is opened with `DECK_PROFILE=update deck`.

## Run

```sh
deck
```

| key | action |
|-----|--------|
| `n` | new session (shell / claude / pi / codex / copilot) |
| `↵` | interactive mode in the preview; `Ctrl+Q` leaves |
| `a` | full tmux attach |
| `r` / `x` / `u` | resume / kill / undo |
| `,` | settings |
| `?` | all keys |
| `q` | quit |

Update check: once a day deck asks GitHub for the latest release tag, so it can say a newer one exists. That one request is deck's only network use; it sends nothing but the request itself (no telemetry, no identifiers). Set `update_check = false` in `config.toml` (or `DECK_UPDATE_CHECK=0`, or the toggle in settings `,`) to disable it and deck makes no outbound request. `auto_update = true` (default false) implies the check and installs a newer release it finds, verified like `deck update`, with no prompt; deck never restarts itself, so restart it to run the new version. Where the install would be refused (unwritable directory, package-managed prefix, `dev` build) nothing is downloaded and the footer says why. The answer is cached in `update-check.json`, shared by all profiles (SPEC §4).

Config: `~/.config/deck/config.toml`. State: `~/.local/share/deck/`. tmux socket: `tmux -L deck`.

## Themes

Nine built-in themes, chosen with `[ui] theme` in `config.toml`, in settings (`,`) or live
from the `t` picker: `empire` (default), `daylight`, `parchment`, `matrix`, `cobalt`,
`gruvbox-dark`, `solarized-dark`, `amber` and `high-contrast`. Your own live in
`~/.config/deck/themes/*.toml` (see SPEC.md §11.6).

## Event hook

One script gets told when a session needs you. Set it in `config.toml`, then turn it on:

```toml
event_hook = "/home/me/bin/deck-notify"
event_hook_default = true
event_hook_events = ["waiting", "error", "ended"]
event_hook_timeout = 3
```

deck runs `script <kind> [fixed args]` (`started`, `resumed`, `waiting`, `idle`, `error`, `ended`,
`killed`) with `DECK_SESSION_*`, `DECK_EVENT_KIND`, `DECK_EVENT_REASON`, `DECK_EVENT_MESSAGE` and
`DECK_EVENT_AT` in the environment and the full JSON payload on stdin (SPEC.md §10, and `?` in deck).
The script path must be absolute (settings refuses to save a relative one, and a relative path in
`config.toml` shows a warning in the health lines); a NUL byte in any exported value is stripped.
A session's own `event_hook_events` replaces the global list. Two notes apply to every script:

- **No retry.** deck keeps no outbox: a script that fails, or is killed after `event_hook_timeout`
  (3 s), has failed. The exit status and an output tail are on the event, in the session detail.
  Keep the script quick and send in the background if the service is slow.
- **Idempotency.** deck spawns once per `(kind, reason)` in an attention episode, but it makes no
  promise across episodes, restarts or two deck processes racing, so a script that must not act
  twice has to deduplicate itself (the examples key on session id, kind and time).

Telegram, with `curl`:

```sh
#!/bin/sh
# deck-notify-telegram: needs TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID in the environment deck runs in.
# No retry from deck: --max-time keeps this inside event_hook_timeout, and a failure is just a non-zero exit.
set -eu
kind=$1
key="${DECK_SESSION_ID:-unknown}-$kind-${DECK_EVENT_AT:-now}"
state="${XDG_STATE_HOME:-$HOME/.local/state}/deck-notify-seen"
mkdir -p "$state"
[ -e "$state/$key" ] && exit 0
text="deck: ${DECK_SESSION_NAME:-session} $kind"
[ -n "${DECK_EVENT_REASON:-}" ] && text="$text ($DECK_EVENT_REASON)"
curl -fsS --max-time 2 \
  --data-urlencode "chat_id=$TELEGRAM_CHAT_ID" \
  --data-urlencode "text=$text" \
  "https://api.telegram.org/bot$TELEGRAM_BOT_TOKEN/sendMessage" >/dev/null
: > "$state/$key"
```

Desktop, with `notify-send`:

```sh
#!/bin/sh
# deck-notify-desktop: a notification per event; deck gives no retry, so a failure is simply lost.
set -eu
kind=$1
case "$kind" in
  error) urgency=critical ;;
  *) urgency=normal ;;
esac
# notify-send replaces the previous bubble with the same id, so a repeat is harmless.
notify-send --urgency="$urgency" --app-name=deck \
  --hint="string:x-canonical-private-synchronous:deck-${DECK_SESSION_ID:-x}" \
  "deck: ${DECK_SESSION_NAME:-session} $kind" "${DECK_EVENT_REASON:-}"
```

## Release

```sh
git tag v0.2.0 && git push origin v0.2.0
```

GitHub Actions ([release.yml](.github/workflows/release.yml)) builds the four binaries,
stamps the tag into `deck --version`, attests the tarballs and `checksums.txt`, and publishes the release.

## Remix, don't PR

This project is **closed to pull requests and open to remix**. It is Apache-2.0: fork it, rewrite it,
let your agent adapt it, and keep your version. I don't review or merge outside pull requests and
there is no maintenance promise. Ideas, bug reports and links to your remix are welcome in
[Discussions](https://github.com/n-orlov/deck/discussions). How to do it: [REMIX.md](REMIX.md).
Background: [Geoff Huntley's remix model](https://ghuntley.com/remix/).

## License

Apache-2.0 (see [LICENSE](LICENSE)).

## Develop

```sh
go build ./cmd/deck                      # local build
ci/run.sh go test -p=1 -count=1 ./...    # full suite in the CI container (~6 min)
```

Behaviour is specified in [SPEC.md](SPEC.md). Phase records live in `docs/reports/`.
