package update

import (
	"bytes"
	"context"
	"crypto/rand"
	"debug/buildinfo"
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
	BaseURL string // overrides DECK_UPDATE_URL and DefaultBaseURL
	Version string // a vX.Y.Z tag to pin; empty means latest
	GOOS    string // default runtime.GOOS
	GOARCH  string // default runtime.GOARCH
	Exe     string // binary to replace; default os.Executable()
	Current string // the running build's version; "dev" refuses (see refuse.go)
	// OnlyIfNewer makes a latest-release install do nothing (Result.Unchanged)
	// when the latest release is not newer than Current. A pinned Version
	// always installs.
	OnlyIfNewer bool
	Client      *http.Client // default: HTTPS-only redirects, 2 min timeout
	// InstallLock is the lock file that serialises installs machine-wide: Run
	// holds an exclusive advisory flock on it from before the latest tag is
	// asked until the install ends. Empty, or a file that cannot be created,
	// means no lock. It is distinct from the daily check's cache lock.
	InstallLock string

	// Repo is the GitHub repository an authenticated gh downloads the release
	// from. Empty means DefaultRepo when the base is DefaultBaseURL, and "no gh
	// download" for any other base (a mirror is fetched over HTTPS).
	Repo string
	// RequireImmutable aborts unless the GitHub release API reports the
	// release immutable even when DECK_REQUIRE_IMMUTABLE=0 opts out (that
	// abort is already the default).
	RequireImmutable bool

	// apiBase is test-only: the release API to ask in place of
	// DefaultAPIBase, whatever the release base is.
	apiBase string
	// root is test-only: the tree the managed prefixes are matched under.
	root string
	// writeFile and rename are the two filesystem steps that can fail after
	// verification; tests inject errors through them.
	writeFile func(name string, data []byte, perm os.FileMode) error
	rename    func(oldpath, newpath string) error
}

// Result reports a successful update.
type Result struct {
	Tag string // the release installed (or, when Unchanged, the latest release)
	// Unchanged is set when OnlyIfNewer found nothing newer: nothing was
	// downloaded and the binary was not touched.
	Unchanged bool
	Path      string // the resolved (symlink-resolved) file that was replaced

	// Immutable is what the GitHub release API said about the installed
	// release: ImmutableVerified, ImmutableNot or ImmutableUnknown (a mirror,
	// or no usable answer). Only ImmutableVerified is a verified immutable
	// release.
	Immutable string
}

// Run downloads, verifies and installs the release over the executable. On
// any error the old binary is untouched and no temporary file remains.
func Run(ctx context.Context, o Options) (Result, error) {
	res, err := run(ctx, o)
	return res, redactErr(err, baseURL(o.BaseURL))
}

func run(ctx context.Context, o Options) (Result, error) {
	o = o.withDefaults()
	pl, err := o.prepare()
	if err != nil {
		return Result{}, err
	}
	// The lock is held to the end of the install, and what the binary on disk
	// is, is decided only once it is held: another deck may have just
	// replaced it.
	unlock, ok := flockPath(ctx, o.InstallLock)
	if !ok {
		return Result{}, fmt.Errorf("wait for another update to finish: %w", ctx.Err())
	}
	defer unlock()
	if pl.tag, err = o.tag(ctx, pl.base); err != nil {
		return Result{}, err
	}
	if o.OnlyIfNewer && o.Version == "" && !IsNewer(pl.tag, o.installedVersion(pl.target)) {
		return Result{Tag: pl.tag, Path: pl.target, Unchanged: true}, nil
	}
	return o.install(ctx, pl)
}

// install runs the part of Run that fetches, verifies and replaces, once the
// tag to install is settled and the install lock is held.
func (o Options) install(ctx context.Context, pl runPlan) (Result, error) {
	// The release's immutability is settled before checksums.txt is trusted,
	// and before anything is downloaded or staged.
	imm, err := o.checkImmutable(ctx, pl.base, pl.tag)
	if err != nil {
		return Result{}, err
	}
	work, err := os.MkdirTemp("", "deck-update-")
	if err != nil {
		return Result{}, fmt.Errorf("create a scratch directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(work) }()
	bin, err := o.fetchBinary(ctx, fetchPlan{releaseURL: pl.base + "/releases/download/" + pl.tag, tag: pl.tag, asset: pl.asset, repo: o.ghRepo(pl.base), work: work})
	if err != nil {
		return Result{}, err
	}
	if err := checkStagedVersion(bin, pl.tag); err != nil {
		return Result{}, err
	}
	if err := o.replace(pl.target, bin); err != nil {
		return Result{}, err
	}
	return Result{Tag: pl.tag, Path: pl.target, Immutable: imm}, nil
}

// runPlan is what Run settles before it downloads anything.
type runPlan struct {
	asset, base, target, tag string
}

// prepare runs Run's refusals and lookups in order: dev build, platform,
// release URL, then install target. The tag to install is settled after.
func (o Options) prepare() (runPlan, error) {
	var pl runPlan
	if err := refuseDev(o.Current); err != nil {
		return pl, err
	}
	var err error
	if pl.asset, err = AssetName(o.GOOS, o.GOARCH); err != nil {
		return pl, err
	}
	pl.base = baseURL(o.BaseURL)
	if err = CheckURL(pl.base); err != nil {
		return pl, err
	}
	pl.target, err = o.checkTarget()
	return pl, err
}

// installedVersion is the version of the binary at target, read from its Go
// build info on disk (never by running it). When that is not a stable release
// tag (a text file, a source build) the running build's version stands.
func (o Options) installedVersion(target string) string {
	if info, err := buildinfo.ReadFile(target); err == nil && stableTag.MatchString(info.Main.Version) {
		return info.Main.Version
	}
	return o.Current
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
	if o.root == "" && testRoot != nil {
		o.root = testRoot()
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
// verifies the archive's SHA-256 checksum, and returns the binary inside it.
func (o Options) fetchBinary(ctx context.Context, p fetchPlan) ([]byte, error) {
	g := findGh(ctx)
	archive, err := o.fetchAsset(ctx, g, p, p.asset, maxArchiveBytes)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", p.asset, err)
	}
	sums, err := o.fetchAsset(ctx, g, p, "checksums.txt", maxChecksumsBytes)
	if err != nil {
		return nil, fmt.Errorf("download checksums.txt: %w", err)
	}
	if err := verifyArchive(archive, sums, p.asset); err != nil {
		return nil, err
	}
	return extractBinary(archive)
}

// checkStagedVersion refuses a staged binary whose embedded module version is
// not the release tag. The version is read from the build info inside the
// bytes (debug/buildinfo); the staged binary is never executed to learn it.
// It runs before anything is written beside the target, so a refusal leaves
// the old binary and its directory untouched.
func checkStagedVersion(bin []byte, tag string) error {
	info, err := buildinfo.Read(bytes.NewReader(bin))
	if err != nil {
		return fmt.Errorf("refusing to install: cannot read the version embedded in the staged binary (release %s): %w", tag, err)
	}
	got := info.Main.Version
	if got == "" || got == "(devel)" {
		return fmt.Errorf("refusing to install: the staged binary embeds no release version (release %s)", tag)
	}
	if got != tag {
		return fmt.Errorf("refusing to install: the staged binary embeds version %q but the release is %s", got, tag)
	}
	return nil
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
