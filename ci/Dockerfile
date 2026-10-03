# Quality-gate tools (#54), pinned, built once in their own stage: golangci-lint
# v2.14.0 needs Go >= 1.26, the product builds on 1.25.x. Bump a version here
# and nowhere else.
FROM golang:1.26.4-trixie AS tools
RUN GOBIN=/out go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 \
    && GOBIN=/out go install golang.org/x/vuln/cmd/govulncheck@v1.8.0

FROM golang:1.25.14-trixie

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
