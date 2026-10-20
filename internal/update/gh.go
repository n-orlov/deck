package update

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DefaultRepo is the GitHub repository releases are built and attested in.
const DefaultRepo = "n-orlov/deck"

// EnvRequireAttestation turns "the attestation could not be checked" from a
// note into an abort, as it does for install.sh.
const EnvRequireAttestation = "DECK_REQUIRE_ATTESTATION"

// Attestation outcomes reported in Result.Attestation.
const (
	AttestationVerified = "verified"
	AttestationSkipped  = "checksum-only"
)

const ghTimeout = 2 * time.Minute

// gh is an authenticated GitHub CLI found on PATH.
type gh struct{ path string }

// findGh returns the gh on PATH when it is authenticated, else nil.
func findGh(ctx context.Context) *gh {
	p, err := exec.LookPath("gh")
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, p, "auth", "status").Run(); err != nil { //nolint:gosec // the user's own gh
		return nil
	}
	return &gh{path: p}
}

func (g *gh) run(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, ghTimeout)
	defer cancel()
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, g.path, args...) //nolint:gosec // the user's own gh
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

// download fetches one release asset into dir through gh and returns its bytes.
func (g *gh) download(ctx context.Context, repo, tag, asset, dir string, limit int64) ([]byte, error) {
	if out, err := g.run(ctx, "release", "download", tag, "-R", repo, "-p", asset, "-D", dir, "--clobber"); err != nil {
		return nil, fmt.Errorf("gh release download %s: %w: %w", asset, err, serverString(strings.TrimSpace(out)))
	}
	name := filepath.Join(dir, asset)
	fi, err := os.Stat(name)
	if err != nil {
		return nil, err
	}
	if fi.Size() > limit {
		return nil, fmt.Errorf("gh release download %s: larger than %d bytes", asset, limit)
	}
	return os.ReadFile(name) //nolint:gosec // a file in our own temp dir
}

// verify runs `gh attestation verify <file> --repo <repo>`.
func (g *gh) verify(ctx context.Context, file, repo string) (string, error) {
	return g.run(ctx, "attestation", "verify", file, "--repo", repo)
}

// errAttestationFailed marks a real verification failure (as opposed to
// "could not check").
var errAttestationFailed = errors.New("attestation verification failed")

// noAttestations is what `gh attestation verify` says for a release that was
// never attested, as opposed to one whose attestation does not verify.
const noAttestations = "no attestations found"

// attest checks the downloaded archive file's build attestation. It returns
// the outcome, or an error when the update must abort. Two cases are "cannot
// verify", a checksum-only outcome unless DECK_REQUIRE_ATTESTATION=1 turns it
// into an abort: an unauthenticated or absent gh, and a release that has no
// attestation at all (gh's "no attestations found": releases built by
// Jenkins carry none). Every other nonzero verification result is a real
// failure and aborts (R294).
func (o Options) attest(ctx context.Context, g *gh, archive string) (string, error) {
	cannot := func(why string) (string, error) {
		if o.RequireAttestation || os.Getenv(EnvRequireAttestation) == "1" {
			return "", fmt.Errorf("%s and %s=1 is set; nothing was installed", why, EnvRequireAttestation)
		}
		return AttestationSkipped, nil
	}
	if g == nil {
		return cannot("the attestation was not checked (no authenticated gh)")
	}
	out, err := g.verify(ctx, archive, DefaultRepo)
	if err == nil {
		return AttestationVerified, nil
	}
	if strings.Contains(out, noAttestations) {
		return cannot("this release has no build attestation")
	}
	return "", fmt.Errorf("%w for %s: %w", errAttestationFailed, filepath.Base(archive), serverString(strings.TrimSpace(out)))
}
