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
	// AttestationRefused is only ever a PlanAttestation answer: the update
	// would be refused, because the attestation cannot be checked and
	// DECK_REQUIRE_ATTESTATION=1 forbids falling back to the checksum.
	AttestationRefused = "refused"
)

// PlanAttestation says, before anything is downloaded, what Run would do
// about the build attestation: AttestationVerified (an authenticated gh is
// present, so the attestation is verified and a failure aborts),
// AttestationSkipped (no authenticated gh: the checksum alone is used, which
// catches corruption only) or AttestationRefused (no authenticated gh and
// DECK_REQUIRE_ATTESTATION=1 or Options.RequireAttestation). It asks the same
// question attest does and touches nothing.
func PlanAttestation(ctx context.Context, o Options) string {
	if findGh(ctx) != nil {
		return AttestationVerified
	}
	if o.RequireAttestation || os.Getenv(EnvRequireAttestation) == "1" {
		return AttestationRefused
	}
	return AttestationSkipped
}

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

// attest checks the downloaded archive file's build attestation. It returns
// the outcome, or an error when the update must abort. Only an unauthenticated
// or absent gh is "cannot verify": a checksum-only outcome, unless
// DECK_REQUIRE_ATTESTATION=1 turns it into an abort. With an authenticated gh
// every nonzero verification result aborts (R294), including a release that
// has no attestation.
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
	return "", fmt.Errorf("%w for %s: %w", errAttestationFailed, filepath.Base(archive), serverString(strings.TrimSpace(out)))
}
