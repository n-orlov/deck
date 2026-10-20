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
#
# Embedded version: deck update refuses a staged binary whose Go build info
# (debug/buildinfo, the main module's version) is not the release tag. Go
# stamps that from a git tag on the commit being built, and the release tag
# does not exist yet (GitHub creates it when the release is published), so
# the script tags HEAD locally for the build and deletes that tag again on
# exit; it is never pushed. Every binary's embedded version is then checked
# against the tag (`go version -m`), so a release deck update would refuse is
# never built.
set -eu

version=${1:?"usage: ci/release-build.sh <tag, e.g. v1.2.3>"}

smoke=$(mktemp -d "${TMPDIR:-/tmp}/deck-release-smoke.XXXXXX")
local_tag=
cleanup() {
    rm -rf "$smoke"
    if [ -n "$local_tag" ]; then
        git tag -d "$local_tag" >/dev/null 2>&1 || true
    fi
}
trap cleanup EXIT

head=$(git rev-parse HEAD)
if tagged=$(git rev-parse -q --verify "refs/tags/${version}^{commit}"); then
    if [ "$tagged" != "$head" ]; then
        echo "ci/release-build.sh: local tag $version points at $tagged, not HEAD $head; delete it first" >&2
        exit 1
    fi
else
    git tag "$version" HEAD
    local_tag=$version
fi

rm -rf dist
mkdir -p dist
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
    os=${target%/*}
    arch=${target#*/}
    CGO_ENABLED=0 GOOS=$os GOARCH=$arch \
        go build -trimpath -ldflags "-s -w -X main.version=${version}" \
        -o "dist/${os}_${arch}/deck" ./cmd/deck
    embedded=$(go version -m "dist/${os}_${arch}/deck" | awk '$1 == "mod" { print $3; exit }')
    if [ "$embedded" != "$version" ]; then
        echo "ci/release-build.sh: dist/${os}_${arch}/deck embeds module version [$embedded], want [$version] (deck update would refuse it)" >&2
        exit 1
    fi
    tar -C "dist/${os}_${arch}" -czf "dist/deck_${os}_${arch}.tar.gz" deck
    rm -r "dist/${os}_${arch}"
done
(cd dist && sha256sum -- *.tar.gz > checksums.txt && cat checksums.txt)

tar -xzf dist/deck_linux_amd64.tar.gz -C "$smoke"
got=$("$smoke/deck" --version)
want="deck ${version} linux/amd64"
if [ "$got" != "$want" ]; then
    echo "ci/release-build.sh: smoke check failed: deck --version printed [$got], want [$want]" >&2
    exit 1
fi
echo "ci/release-build.sh: smoke check ok: $got"
