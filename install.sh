#!/bin/sh
# Install deck (https://github.com/n-orlov/deck) from its latest GitHub release.
#
#   curl -fsSL https://raw.githubusercontent.com/n-orlov/deck/main/install.sh | sh
#
# Env:  DECK_VERSION=vX.Y.Z   pin a release (default: latest)
#       DECK_INSTALL_DIR=DIR  where to put the binary (default: ~/.local/bin)
#       DECK_REQUIRE_ATTESTATION=1  abort when the build attestation cannot be checked
#
# Integrity: the asset is checked against checksums.txt, which is downloaded
# from the SAME release as the asset. That catches a corrupt or truncated
# download, not tampering: whoever can replace the asset can replace
# checksums.txt as well. All downloads are HTTPS-only (--proto =https).
#
# Provenance: releases are attested by release.yml. After the checksum check an
# authenticated `gh` runs `gh attestation verify --repo n-orlov/deck` on the
# archive, and a verification FAILURE always aborts with nothing installed.
# Without a usable `gh`, or for a release published before attestations
# existed, one note says only the checksum was verified and the install goes
# on -- unless DECK_REQUIRE_ATTESTATION=1, which aborts instead.
#
# Uses an authenticated `gh` when present, otherwise plain curl. Linux and macOS only; on Windows run it inside WSL.
set -eu

REPO=n-orlov/deck
version="${DECK_VERSION:-latest}"
dir="${DECK_INSTALL_DIR:-$HOME/.local/bin}"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$os" in
  linux | darwin) ;;
  *) echo "deck: unsupported OS '$os' -- deck needs tmux; on Windows use WSL" >&2; exit 1 ;;
esac
case "$arch" in
  x86_64 | amd64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) echo "deck: unsupported architecture '$arch'" >&2; exit 1 ;;
esac
asset="deck_${os}_${arch}.tar.gz"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

fetch() { # fetch <asset> into $tmp
  if command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then
    if [ "$version" = latest ]; then
      gh release download -R "$REPO" -p "$1" -D "$tmp" --clobber
    else
      gh release download "$version" -R "$REPO" -p "$1" -D "$tmp" --clobber
    fi
  else
    if [ "$version" = latest ]; then
      url="https://github.com/$REPO/releases/latest/download/$1"
    else
      url="https://github.com/$REPO/releases/download/$version/$1"
    fi
    curl --proto =https --tlsv1.2 -fsSL -o "$tmp/$1" "$url"
  fi
}

echo "deck: fetching $version $asset"
fetch "$asset"
fetch checksums.txt

# Verify: sha256sum on Linux, shasum on macOS.
if command -v sha256sum >/dev/null 2>&1; then
  (cd "$tmp" && grep " $asset\$" checksums.txt | sha256sum -c --quiet -)
else
  (cd "$tmp" && grep " $asset\$" checksums.txt | shasum -a 256 -c --quiet -)
fi

# Attestation: abort on a real failure; "cannot verify" only notes (or aborts
# under DECK_REQUIRE_ATTESTATION=1).
cannot_verify() { # cannot_verify <why>
  if [ "${DECK_REQUIRE_ATTESTATION:-}" = 1 ]; then
    echo "deck: aborting -- $1 and DECK_REQUIRE_ATTESTATION=1 is set; nothing was installed" >&2
    exit 1
  fi
  echo "deck: note -- only the checksum was verified; $1" >&2
}
if command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then
  if att_out=$(gh attestation verify "$tmp/$asset" --repo "$REPO" 2>&1); then
    echo "deck: attestation verified for $asset"
  else
    case "$att_out" in
      *"no attestations found"*) cannot_verify "this release has no attestation" ;;
      *)
        printf '%s\n' "$att_out" >&2
        echo "deck: attestation verification FAILED for $asset -- nothing was installed" >&2
        exit 1
        ;;
    esac
  fi
else
  cannot_verify "the attestation was not checked (no authenticated gh)"
fi

tar -xzf "$tmp/$asset" -C "$tmp"
mkdir -p "$dir"
# Copy then rename: replacing a running deck in place fails with "text file busy".
cp "$tmp/deck" "$dir/deck.new"
chmod 0755 "$dir/deck.new"
mv -f "$dir/deck.new" "$dir/deck"

echo "deck: installed $("$dir/deck" --version) -> $dir/deck"
case ":$PATH:" in
  *":$dir:"*) ;;
  *) echo "deck: note -- $dir is not on your PATH" >&2 ;;
esac
if ! command -v tmux >/dev/null 2>&1; then
  echo "deck: note -- tmux >= 3.2 is required and was not found on PATH" >&2
fi
