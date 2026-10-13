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
	Client  *http.Client // default: HTTPS-only redirects, 2 min timeout

	// writeFile and rename are the two filesystem steps that can fail after
	// verification; tests inject errors through them.
	writeFile func(name string, data []byte, perm os.FileMode) error
	rename    func(oldpath, newpath string) error
}

// Result reports a successful update.
type Result struct {
	Tag  string // the release installed
	Path string // the resolved (symlink-resolved) file that was replaced
}

// Run downloads, verifies and installs the release over the executable. On
// any error the old binary is untouched and no temporary file remains.
func Run(ctx context.Context, o Options) (Result, error) {
	o = o.withDefaults()
	asset, err := AssetName(o.GOOS, o.GOARCH)
	if err != nil {
		return Result{}, err
	}
	base := baseURL(o.BaseURL)
	if err := CheckURL(base); err != nil {
		return Result{}, err
	}
	target, err := resolveExecutable(o.Exe)
	if err != nil {
		return Result{}, err
	}
	tag, err := o.tag(ctx, base)
	if err != nil {
		return Result{}, err
	}
	bin, err := o.fetchBinary(ctx, base+"/releases/download/"+tag, asset)
	if err != nil {
		return Result{}, err
	}
	if err := o.replace(target, bin); err != nil {
		return Result{}, err
	}
	return Result{Tag: tag, Path: target}, nil
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

// fetchBinary downloads the archive and checksums.txt from one release,
// verifies the archive and returns the binary inside it.
func (o Options) fetchBinary(ctx context.Context, releaseURL, asset string) ([]byte, error) {
	archive, err := get(ctx, o.Client, releaseURL+"/"+asset, maxArchiveBytes)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", asset, err)
	}
	sums, err := get(ctx, o.Client, releaseURL+"/checksums.txt", maxChecksumsBytes)
	if err != nil {
		return nil, fmt.Errorf("download checksums.txt: %w", err)
	}
	if err := verifyArchive(archive, sums, asset); err != nil {
		return nil, err
	}
	return extractBinary(archive)
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
