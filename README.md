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
- Check: `deck --version`. Update: rerun the same line.

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

## Develop

```sh
go build ./cmd/deck                      # local build
ci/run.sh go test -p=1 -count=1 ./...    # full suite in the CI container (~6 min)
```

Behaviour is specified in [SPEC.md](SPEC.md). Phase records live in `docs/reports/`.
