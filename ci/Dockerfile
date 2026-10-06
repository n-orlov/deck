# Quality-gate tools (#54), pinned, built once in their own stage: golangci-lint
# v2.14.0 needs Go >= 1.26, the product builds on 1.25.x. Bump a version here
# and nowhere else. Every FROM is pinned by digest (a tag can be re-pushed); the
# tag stays in the reference and in the comment above it, so a bump changes both.
# golang:1.26.4-trixie
FROM golang:1.26.4-trixie@sha256:68b7145ec43d1820b9a56704554b53d1520aa2a15cb5233e374188a31b2a1bce AS tools
RUN GOBIN=/out go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 \
    && GOBIN=/out go install golang.org/x/vuln/cmd/govulncheck@v1.8.0

# golang:1.25.14-trixie
FROM golang:1.25.14-trixie@sha256:2c4c60ef415fbfa5e90300722293bef36c5e63fae17570ce18f580af933dbd73

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        git \
        tmux \
    && rm -rf /var/lib/apt/lists/*

COPY --from=tools /out/golangci-lint /out/govulncheck /usr/local/bin/

# Trivy 0.72.0, pinned by digest.
COPY --from=public.ecr.aws/aquasecurity/trivy@sha256:cffe3f5161a47a6823fbd23d985795b3ed72a4c806da4c4df16266c02accdd6f /usr/local/bin/trivy /usr/local/bin/trivy

ENV GOMODCACHE=/go-cache/mod \
    GOCACHE=/go-cache/build \
    GOLANGCI_LINT_CACHE=/go-cache/golangci

RUN mkdir -p "$GOMODCACHE" "$GOCACHE" "$GOLANGCI_LINT_CACHE" \
    && chown -R 1000:1000 /go-cache

# Non-root by default (Trivy DS-0002). ci/run.sh still passes --user uid:gid.
USER 1000:1000

VOLUME ["/go-cache"]
WORKDIR /w
