#!/bin/sh
# Run a command in a throwaway sibling container that has the project toolchain
# (Go + tmux). Works both inside a ralphd job container and directly on the host.
#
#   ci/run.sh go test ./...
#   ci/run.sh sh -c 'cd .spike && go test ./...'
#
# Invariants that matter (see ci/SPIKE.md):
#   - the bind source must be a HOST path, because siblings run on the host daemon;
#     inside a ralphd job that path is $RALPHD_HOST_WORKSPACE, not /workspace
#   - the sibling runs as the calling uid/gid, so it never leaves root-owned files
#   - the module/build cache lives in a named volume shared across runs, so warm
#     invocations skip dependency downloads
set -eu

workspace=${RALPHD_HOST_WORKSPACE:-$PWD}
image=${DECK_CI_IMAGE:-deck-ci:local}
volume=${DECK_CI_CACHE_VOLUME:-deck-go-cache}

if ! docker image inspect "$image" >/dev/null 2>&1; then
    echo "error: image $image not found; build it with:" >&2
    echo "  docker build -t $image -f ci/Dockerfile ci" >&2
    exit 1
fi

# Deliberately shared across runs: the cache is the whole point, so it is named
# after the toolchain rather than a run id. Remove it to force a cold build.
if ! docker volume inspect "$volume" >/dev/null 2>&1; then
    docker volume create --label deck.cache=go "$volume" >/dev/null
fi

set -- docker run --rm \
    --user "$(id -u):$(id -g)" \
    --workdir /w \
    --mount "type=bind,src=$workspace,dst=/w" \
    --mount "type=volume,src=$volume,dst=/go-cache" \
    ${RALPHD_RUN_ID:+--label ralphd.run=$RALPHD_RUN_ID} \
    ${RALPHD_RUN_ID:+--label ralphd.role=sibling} \
    "$image" "$@"

exec "$@"
