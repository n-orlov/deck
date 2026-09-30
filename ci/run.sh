#!/bin/sh
# Run a command in a throwaway sibling container that has the project toolchain
# (Go + tmux). Works both inside a ralphd job container and directly on the host.
#
#   ci/run.sh go test ./...
#   ci/run.sh sh -c 'cd .spike && go test ./...'
#   DECK_GODOG_PATHS=session_groups.feature ci/run.sh go test ./features/ -run TestFeatures
#
# Invariants that matter (see ci/SPIKE.md):
#   - the bind source must be a HOST path, because siblings run on the host daemon;
#     inside a ralphd job that path is $RALPHD_HOST_WORKSPACE, not /workspace
#   - the sibling runs as the calling uid/gid, so it never leaves root-owned files
#   - the module/build cache lives in a named volume shared across runs, so warm
#     invocations skip dependency downloads
#   - a sibling starts with a fresh environment, so any DECK_* variable the caller
#     set in front of ci/run.sh is forwarded explicitly (see below); without that,
#     selectors like DECK_GODOG_PATHS were silently dropped and the whole feature
#     suite ran instead of the named files
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

# The sibling command is assembled by appending the docker prefix *after* the
# caller's own argv and then rotating that argv to the end, so the forwarded `env`
# assignments can sit between the image name and the command being run.
user_argc=$#

# Every command run this way gets task cure-01-01 (R158)'s tmux guard put
# ahead of the real tmux on PATH first: a symlink at tmux-guard-bin/tmux
# that refuses "-L deck"/"-L deck-*" before it ever reaches the real
# binary (see ci/tmux-guard.sh), so no fixture run through ci/run.sh --
# including the mandated full `go test -p=1 -count=1 ./...` -- can land a
# command against the operator's real tmux server, however it is named.
guard_setup='set -e
real_tmux=$(command -v tmux)
real_bin_dir=$(dirname "$real_tmux")
guard_dir="/tmp/tmux-guard-bin-$$"
mkdir -p "$guard_dir"
# Mirror every OTHER binary in tmux'\''s own directory into guard_dir
# untouched (some fixtures deliberately restrict PATH to just
# dirname(tmux) so a pane launch still finds its shell -- see
# internal/service/resume_test.go'\''s TestResumeFailsOnAgentBinaryNotOnPath),
# and only replace the tmux entry itself with the guard.
for f in "$real_bin_dir"/*; do
  name=$(basename "$f")
  if [ "$name" = "tmux" ]; then
    ln -sf /w/ci/tmux-guard.sh "$guard_dir/tmux"
  else
    ln -sf "$f" "$guard_dir/$name"
  fi
done
export DECK_TEST_REAL_TMUX="$real_tmux"
export DECK_TEST_TMUX_GUARD_LOG="${DECK_TEST_TMUX_GUARD_LOG:-/tmp/tmux-guard.log}"
export PATH="$guard_dir:$PATH"
exec "$@"'

set -- "$@" docker run --rm \
    --user "$(id -u):$(id -g)" \
    --workdir /w \
    --mount "type=bind,src=$workspace,dst=/w" \
    --mount "type=volume,src=$volume,dst=/go-cache" \
    ${RALPHD_RUN_ID:+--label ralphd.run=$RALPHD_RUN_ID} \
    ${RALPHD_RUN_ID:+--label ralphd.role=sibling} \
    "$image" sh -c "$guard_setup" sh

# Forward every DECK_* variable present in the caller's environment, as an `env`
# prefix on the command itself. This makes
#   DECK_GODOG_PATHS=x ci/run.sh go test ./features/ -run TestFeatures
# behave exactly like the longhand form
#   ci/run.sh env DECK_GODOG_PATHS=x go test ./features/ -run TestFeatures
# which keeps working unchanged. Nothing is added when no DECK_* variable is set.
forwarded=$(env | sed -n 's/^\(DECK_[A-Za-z_][A-Za-z0-9_]*\)=.*/\1/p' | sort -u)
if [ -n "$forwarded" ]; then
    set -- "$@" env
    for name in $forwarded; do
        eval "set -- \"\$@\" \"$name=\${$name}\""
    done
fi

rotated=0
while [ "$rotated" -lt "$user_argc" ]; do
    head=$1
    shift
    set -- "$@" "$head"
    rotated=$((rotated + 1))
done

exec "$@"
