#!/bin/sh
# jenkins-image.sh: on a Jenkins agent, build the deck-ci:local image that
# ci/run.sh uses and make the Go cache volume ($DECK_CI_CACHE_VOLUME) usable
# by the agent's uid. Shared by Jenkinsfile and Jenkinsfile.release.
set -eu

: "${DECK_CI_CACHE_VOLUME:?set DECK_CI_CACHE_VOLUME}"

docker build -t deck-ci:local -f ci/Dockerfile ci
# The image owns /go-cache as 1000:1000; the agent runs as its own uid, so
# its cache volume is chowned to it once.
docker volume inspect "$DECK_CI_CACHE_VOLUME" >/dev/null 2>&1 ||
  docker volume create --label deck.cache=go "$DECK_CI_CACHE_VOLUME" >/dev/null
me="$(id -u):$(id -g)"
owner=$(docker run --rm --user 0 --entrypoint stat \
  --mount "type=volume,src=$DECK_CI_CACHE_VOLUME,dst=/go-cache" deck-ci:local -c %u:%g /go-cache)
if [ "$owner" != "$me" ]; then
  echo "chowning $DECK_CI_CACHE_VOLUME from $owner to $me"
  docker run --rm --user 0 --entrypoint chown \
    --mount "type=volume,src=$DECK_CI_CACHE_VOLUME,dst=/go-cache" deck-ci:local -R "$me" /go-cache
fi
