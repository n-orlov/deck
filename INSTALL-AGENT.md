# Installing deck: a guide for coding agents

This file is for a coding agent (Claude Code, Codex, pi, Copilot or any other) that a user has
asked to install deck. It does what [`install.sh`](install.sh) does, one command at a time, so
nothing runs blind: run each step, check its result, and stop on the first failure.

Rules for the agent:

- **Never use `sudo`**, and never install outside the user's home directory, unless the user
  says so first. The default install directory is `~/.local/bin`.
- **Stop and tell the user** whenever a step says "stop". Do not work around a failed check
  (a checksum mismatch, an unconfirmed immutable release) by skipping it.
- Run the steps in one shell session: later steps use the variables set by earlier ones.
- Tell the user what you are doing as you go, and finish with the report in step 9.

deck runs on **Linux and macOS, amd64 and arm64**. There is no Windows build (deck needs
tmux); on Windows, do all of this inside WSL.

## 1. Detect the platform

```sh
os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
echo "$os $arch"
```

Map the result to the release asset:

| `uname -s` | `uname -m` | asset |
|---|---|---|
| `Linux` | `x86_64` or `amd64` | `deck_linux_amd64.tar.gz` |
| `Linux` | `aarch64` or `arm64` | `deck_linux_arm64.tar.gz` |
| `Darwin` | `x86_64` or `amd64` | `deck_darwin_amd64.tar.gz` |
| `Darwin` | `aarch64` or `arm64` | `deck_darwin_arm64.tar.gz` |

```sh
case "$arch" in x86_64 | amd64) arch=amd64 ;; aarch64 | arm64) arch=arm64 ;; esac
asset="deck_${os}_${arch}.tar.gz"
echo "$asset"
```

Anything else (another OS, `armv7l`, `i686`, ...): **stop**, there is no build for it.

## 2. Check the prerequisites

```sh
tmux -V                       # needs tmux 3.2 or newer
dir="${DECK_INSTALL_DIR:-$HOME/.local/bin}"
mkdir -p "$dir" && test -w "$dir" && echo "writable: $dir"
case ":$PATH:" in *":$dir:"*) echo "on PATH" ;; *) echo "NOT on PATH: $dir" ;; esac
command -v curl; command -v sha256sum || command -v shasum
```

- tmux missing or older than 3.2: deck installs but will not run. Tell the user and suggest
  their package manager (`brew install tmux`, `sudo apt install tmux`, `sudo dnf install tmux`);
  do not run a `sudo` command yourself without asking.
- Directory not writable: **stop** and ask the user where to install. Do not reach for `sudo`.
- Not on `PATH`: carry on, and include it in the report in step 9.
- deck's agents (`claude`, `pi`, `codex`, `copilot`) only show up in deck if they are on `PATH`.

## 3. Pick the release

Use the version the user asked for (`vX.Y.Z`), else the latest release:

```sh
tag="${DECK_VERSION:-}"
[ -n "$tag" ] || tag=$(curl --proto =https --tlsv1.2 -fsSL \
  https://api.github.com/repos/n-orlov/deck/releases/latest |
  sed -n 's/^  "tag_name": "\(.*\)",$/\1/p')
echo "$tag"
```

If `tag` is empty (offline, rate-limited): **stop** and tell the user. An authenticated `gh` works
too: `gh release view -R n-orlov/deck --json tagName -q .tagName`.

## 4. Check the release is immutable

An [immutable release](https://docs.github.com/en/code-security/supply-chain-security/understanding-your-software-supply-chain/immutable-releases)
cannot have its assets replaced after publication. `deck update` refuses a release that GitHub
does not report immutable, and this guide applies the same rule:

```sh
curl --proto =https --tlsv1.2 -fsSL \
  "https://api.github.com/repos/n-orlov/deck/releases/tags/$tag" | grep '"immutable":'
```

- `"immutable": true`: carry on.
- `"immutable": false`, the field missing, or the request failing: **stop**. Tell the user the
  release could not be confirmed immutable (every release up to v0.2.18 is not), and install it
  only if they explicitly accept that. This is the same opt-out as `DECK_REQUIRE_IMMUTABLE=0` for
  `deck update`: the checksum in step 6 is then the only integrity check.

## 5. Download the archive and its checksums

```sh
tmp=$(mktemp -d)
base="https://github.com/n-orlov/deck/releases/download/$tag"
curl --proto =https --tlsv1.2 -fsSL -o "$tmp/$asset" "$base/$asset"
curl --proto =https --tlsv1.2 -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt"
ls -l "$tmp"
```

With an authenticated `gh`, `gh release download "$tag" -R n-orlov/deck -p "$asset" -p checksums.txt -D "$tmp"`
does the same.

## 6. Verify the SHA-256 checksum

```sh
cd "$tmp" && grep " $asset\$" checksums.txt | sha256sum -c -          # Linux
cd "$tmp" && grep " $asset\$" checksums.txt | shasum -a 256 -c -      # macOS
```

The line must end in `OK`. Anything else (a mismatch, no line for `$asset`): **stop**, install
nothing, and delete `$tmp`. `checksums.txt` comes from the same release as the archive, so this
catches a corrupt or truncated download; step 4 is what guards against a replaced release.

## 7. Install

```sh
tar -xzf "$tmp/$asset" -C "$tmp"
cp "$tmp/deck" "$dir/deck.new"
chmod 0755 "$dir/deck.new"
mv -f "$dir/deck.new" "$dir/deck"     # rename, not overwrite: a running deck would be "text file busy"
rm -rf "$tmp"
```

## 8. Verify

```sh
"$dir/deck" --version
```

It prints `deck <tag> <os>/<arch>`, for example `deck v0.2.25 linux/amd64`. The tag must be the
one from step 3; if it is not, or the command fails, tell the user.

## 9. Report

Tell the user, in a few lines:

- the version and platform installed, and the path (`$dir/deck`);
- whether the release was confirmed immutable, or installed on their say-so without it;
- anything still to do: `$dir` not on `PATH` (and the line to add to their shell profile, e.g.
  `export PATH="$HOME/.local/bin:$PATH"`), tmux missing or too old;
- that `deck` starts it, and `?` inside deck lists every key.

## Update

deck updates itself, with the same checksum and immutability rules as above:

```sh
deck update --check            # only report whether a newer release exists
deck update                    # install the latest release
deck update --version vX.Y.Z   # pin a release (a downgrade is allowed)
```

It refuses a dev build, a package-managed install (Homebrew, apt, Nix, snap) and an unwritable
directory; report the refusal to the user rather than working around it. Restart a running deck
afterwards. Re-running steps 3 to 8 also updates it.

## Uninstall

Ask before deleting anything beyond the binary: the data directory holds the user's session
history.

```sh
rm "${DECK_INSTALL_DIR:-$HOME/.local/bin}/deck"
# Only if the user wants their settings and history gone too:
rm -rf "${XDG_CONFIG_HOME:-$HOME/.config}/deck" "${XDG_DATA_HOME:-$HOME/.local/share}/deck" \
       "${XDG_STATE_HOME:-$HOME/.local/state}/deck" "${XDG_CACHE_HOME:-$HOME/.cache}/deck"
```

Sessions deck started live on its own tmux socket; `tmux -L deck ls` shows any still running
(`tmux -L deck-<name> ls` for a named profile), and uninstalling does not stop them.
