package update

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DevVersion is the version string of a build that was not cut from a release.
const DevVersion = "dev"

// ManagedPrefixes is the finite list of directory prefixes owned by a package
// manager. A binary at or under one of them is updated by that manager, never
// by deck. SPEC §13.1 ("Update refusals") names the same list, and a test
// holds the two equal. A bare /usr is deliberately absent: it would also
// refuse a manual install under /usr/local/bin.
var ManagedPrefixes = []string{
	"/usr/bin", "/usr/sbin", "/bin", "/sbin", "/usr/lib", "/usr/libexec",
	"/opt/homebrew", "/usr/local/Cellar", "/home/linuxbrew", "/nix/store", "/snap",
}

// Refusal is returned, with nothing changed, when the update must not go
// ahead: a dev build, a package-managed binary or an unwritable install
// directory. deck never escalates privileges to get past one.
type Refusal struct {
	Reason  string // RefusedDev, RefusedManaged or RefusedUnwritable
	Message string
}

// The three refusal reasons.
const (
	RefusedDev        = "dev-build"
	RefusedManaged    = "managed-prefix"
	RefusedUnwritable = "unwritable-dir"
)

func (r *Refusal) Error() string { return r.Message }

// IsRefusal reports whether err is a Refusal.
func IsRefusal(err error) bool {
	var r *Refusal
	return errors.As(err, &r)
}

func refuse(reason, format string, args ...any) error {
	return &Refusal{Reason: reason, Message: fmt.Sprintf(format, args...)}
}

// refuseDev refuses a build that is not a release.
func refuseDev(current string) error {
	if current != DevVersion {
		return nil
	}
	return refuse(RefusedDev, "this is a dev build (version %q), not a release: it cannot be updated in place; install a release with install.sh instead", DevVersion)
}

// managedPrefix is the managed prefix path lies at or under, or "". root is
// stripped first (empty in production; tests relocate the tree under a
// temporary directory).
func managedPrefix(path, root string) string {
	p := filepath.ToSlash(filepath.Clean(path))
	if root != "" {
		p = "/" + strings.TrimPrefix(strings.TrimPrefix(p, filepath.ToSlash(filepath.Clean(root))), "/")
	}
	for _, m := range ManagedPrefixes {
		if p == m || strings.HasPrefix(p, m+"/") {
			return m
		}
	}
	return ""
}

func refuseManaged(path, prefix string) error {
	return refuse(RefusedManaged, "%s is under %s, which a package manager owns: update deck with that package manager (brew upgrade, apt, nix, snap) instead", path, prefix)
}

// refuseUnwritable probes dir by creating and removing a file in it.
func refuseUnwritable(dir, target string) error {
	tmp, err := tempBeside(target)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Clean(tmp), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return nil
	}
	return refuse(RefusedUnwritable, "cannot write to the install directory %s (%v): re-run with sudo (sudo deck update), or reinstall to ~/.local/bin with DECK_INSTALL_DIR=~/.local/bin using install.sh", dir, err)
}

// checkTarget resolves exe and refuses it when it sits under a managed prefix
// (before or after resolving symlinks) or in a directory deck cannot write.
func (o Options) checkTarget() (string, error) {
	if o.Exe == "" {
		return "", fmt.Errorf("cannot locate the running deck binary")
	}
	if m := managedPrefix(o.Exe, o.root); m != "" {
		return "", refuseManaged(o.Exe, m)
	}
	target, err := resolveExecutable(o.Exe)
	if err != nil {
		return "", err
	}
	root := o.root
	if root != "" {
		if r, err := filepath.EvalSymlinks(root); err == nil {
			root = r
		}
	}
	if m := managedPrefix(target, root); m != "" {
		return "", refuseManaged(target, m)
	}
	if err := refuseUnwritable(filepath.Dir(target), target); err != nil {
		return "", err
	}
	return target, nil
}
