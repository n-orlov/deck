#!/bin/sh
# ci/release-build.sh — builds the release archives for one tag into dist/:
# deck_<os>_<arch>.tar.gz for linux/darwin x amd64/arm64, plus checksums.txt
# (sha256sum format, what install.sh verifies against), then smoke-checks the
# linux/amd64 binary: `deck --version` must print exactly
# "deck <tag> linux/amd64". Run through ci/run.sh, so releases are built by
# the Go pinned in ci/Dockerfile (Jenkinsfile.release, the deck/release job):
#
#   ci/run.sh ci/release-build.sh v1.2.3
#
# The smoke check runs the binary, so the calling host/container must be
# linux/amd64.
set -eu

version=${1:?"usage: ci/release-build.sh <tag, e.g. v1.2.3>"}

rm -rf dist
mkdir -p dist
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
    os=${target%/*}
    arch=${target#*/}
    CGO_ENABLED=0 GOOS=$os GOARCH=$arch \
        go build -trimpath -ldflags "-s -w -X main.version=${version}" \
        -o "dist/${os}_${arch}/deck" ./cmd/deck
    tar -C "dist/${os}_${arch}" -czf "dist/deck_${os}_${arch}.tar.gz" deck
    rm -r "dist/${os}_${arch}"
done
(cd dist && sha256sum -- *.tar.gz > checksums.txt && cat checksums.txt)

smoke=$(mktemp -d "${TMPDIR:-/tmp}/deck-release-smoke.XXXXXX")
trap 'rm -rf "$smoke"' EXIT
tar -xzf dist/deck_linux_amd64.tar.gz -C "$smoke"
got=$("$smoke/deck" --version)
want="deck ${version} linux/amd64"
if [ "$got" != "$want" ]; then
    echo "ci/release-build.sh: smoke check failed: deck --version printed [$got], want [$want]" >&2
    exit 1
fi
echo "ci/release-build.sh: smoke check ok: $got"
