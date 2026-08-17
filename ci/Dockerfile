FROM golang:1.25-trixie

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        git \
        tmux \
    && rm -rf /var/lib/apt/lists/*

ENV GOMODCACHE=/go-cache/mod \
    GOCACHE=/go-cache/build

RUN mkdir -p "$GOMODCACHE" "$GOCACHE" \
    && chown -R 1000:1000 /go-cache

VOLUME ["/go-cache"]
WORKDIR /w
