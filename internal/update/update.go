package update

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
)

// Options configures one update. The zero value updates the running binary
// to the latest release from the default (or DECK_UPDATE_URL) base.
type Options struct {
	BaseURL string       // overrides DECK_UPDATE_URL and DefaultBaseURL
	Version string       // a vX.Y.Z tag to pin; empty means latest
	GOOS    string       // default runtime.GOOS
	GOARCH  string       // default runtime.GOARCH
	Exe     string       // binary to replace; default os.Executable()
	Current string       // the running build's version; "dev" refuses (see refuse.go)
	Client  *http.Client // default: HTTPS-only redirects, 2 min timeout

	// Repo is the GitHub repository an authenticated gh downloads the release
	// from. Empty means DefaultRepo when the base is DefaultBaseURL, and "no gh
	// download" for any other base (a mirror is fetched over HTTPS).
	Repo string
	// RequireAttestation aborts when the attestation cannot be checked, like
	// DECK_REQUIRE_ATTESTATION=1.
	RequireAttestation bool

	// root is test-only: the tree the managed prefixes are matched under.
	root string
	// writeFile and rename are the two filesystem steps that can fail after
	// verification; tests inject errors through them.
	writeFile func(name string, data []byte, perm os.FileMode) error
	rename    func(oldpath, newpath string) error
}

// Result reports a successful update.
type Result struct {
	Tag  string // the release installed
	Path string // the resolved (symlink-resolved) file that was replaced

	// Attestation is AttestationVerified, or AttestationSkipped when only the
	// checksum was verified (no usable gh, or a release without attestation).
	Attestation string
}

// Run downloads, verifies and installs the release over the executable. On
// any error the old binary is untouched and no temporary file remains.
func Run(ctx context.Context, o Options) (Result, error) {
	o = o.withDefaults()
	if err := refuseDev(o.Current); err != nil {
		return Result{}, err
	}
	asset, err := AssetName(o.GOOS, o.GOARCH)
	if err != nil {
		return Result{}, err
	}
	base := baseURL(o.BaseURL)
	if err := CheckURL(base); err != nil {
		return Result{}, err
	}
	target, err := o.checkTarget()
	if err != nil {
		return Result{}, err
	}
	tag, err := o.tag(ctx, base)
	if err != nil {
		return Result{}, err
	}
	work, err := os.MkdirTemp("", "deck-update-")
	if err != nil {
		return Result{}, fmt.Errorf("create a scratch directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(work) }()
	bin, att, err := o.fetchBinary(ctx, fetchPlan{releaseURL: base + "/releases/download/" + tag, tag: tag, asset: asset, repo: o.ghRepo(base), work: work})
	if err != nil {
		return Result{}, err
	}
	if err := o.replace(target, bin); err != nil {
		return Result{}, err
	}
	return Result{Tag: tag, Path: target, Attestation: att}, nil
}

func (o Options) withDefaults() Options {
	if o.GOOS == "" {
		o.GOOS = runtime.GOOS
	}
	if o.GOARCH == "" {
		o.GOARCH = runtime.GOARCH
	}
	if o.Client == nil {
		o.Client = newClient()
	}
	if o.writeFile == nil {
		o.writeFile = writeExclusive
	}
	if o.rename == nil {
		o.rename = os.Rename
	}
	if o.Exe == "" {
		o.Exe, _ = os.Executable()
	}
	return o
}

// resolveExecutable is the resolved file behind exe: the replacement is written
// beside it, never beside a symlink that points at it.
func resolveExecutable(exe string) (string, error) {
	if exe == "" {
		return "", fmt.Errorf("cannot locate the running deck binary")
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("resolve the running deck binary: %w", err)
	}
	return resolved, nil
}

func (o Options) tag(ctx context.Context, base string) (string, error) {
	if o.Version == "" {
		return LatestTag(ctx, o.Client, base)
	}
	if !stableTag.MatchString(o.Version) {
		return "", fmt.Errorf("version %q is not a stable vX.Y.Z tag", o.Version)
	}
	return o.Version, nil
}

// ghRepo is the repository to download through gh, or "" for HTTPS only.
func (o Options) ghRepo(base string) string {
	if o.Repo != "" {
		return o.Repo
	}
	if base == DefaultBaseURL {
		return DefaultRepo
	}
	return ""
}

// fetchPlan is where one release's files come from and are staged.
type fetchPlan struct {
	releaseURL, tag, asset, repo, work string
}

// fetchAsset downloads one file, through an authenticated gh when there is
// one and a repo to ask it about, else (or when gh fails) over HTTPS.
func (o Options) fetchAsset(ctx context.Context, g *gh, p fetchPlan, name string, limit int64) ([]byte, error) {
	if g != nil && p.repo != "" {
		if data, err := g.download(ctx, p.repo, p.tag, name, p.work, limit); err == nil {
			return data, nil
		}
	}
	return get(ctx, o.Client, p.releaseURL+"/"+name, limit)
}

// fetchBinary downloads the archive and checksums.txt from one release,
// verifies the archive's checksum and build attestation, and returns the
// binary inside it with the attestation outcome.
func (o Options) fetchBinary(ctx context.Context, p fetchPlan) ([]byte, string, error) {
	g := findGh(ctx)
	archive, err := o.fetchAsset(ctx, g, p, p.asset, maxArchiveBytes)
	if err != nil {
		return nil, "", fmt.Errorf("download %s: %w", p.asset, err)
	}
	sums, err := o.fetchAsset(ctx, g, p, "checksums.txt", maxChecksumsBytes)
	if err != nil {
		return nil, "", fmt.Errorf("download checksums.txt: %w", err)
	}
	if err := verifyArchive(archive, sums, p.asset); err != nil {
		return nil, "", err
	}
	file := filepath.Join(p.work, p.asset)
	if err := os.WriteFile(file, archive, 0o600); err != nil {
		return nil, "", fmt.Errorf("stage %s: %w", p.asset, err)
	}
	att, err := o.attest(ctx, g, file)
	if err != nil {
		return nil, "", err
	}
	bin, err := extractBinary(archive)
	return bin, att, err
}

// replace writes bin beside target and renames it over target. The
// temporary file is removed on any failure, so the directory is unchanged.
func (o Options) replace(target string, bin []byte) error {
	tmp, err := tempBeside(target)
	if err != nil {
		return err
	}
	if err := o.writeFile(tmp, bin, 0o755); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("write the new binary beside %s: %w", target, err)
	}
	if err := o.rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace %s: %w", target, err)
	}
	return nil
}

func tempBeside(target string) (string, error) {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(target), "."+filepath.Base(target)+".update-"+hex.EncodeToString(b[:])), nil
}

// writeExclusive creates name (failing if it exists) with perm, regardless
// of umask, and writes data to it.
func writeExclusive(name string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(filepath.Clean(name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Chmod(name, perm)
}
