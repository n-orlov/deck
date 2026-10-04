package features

import (
	"os"
	"path/filepath"
	"strings"
)

// envAbsPath returns the cleaned absolute path exported in the named
// environment variable, or "" when it is unset, blank or not absolute. The
// suite wrappers (ci/suite.sh, the CI workflow) always export absolute paths
// for these diagnostic knobs, so a relative value is a misconfiguration the
// callers treat the same as unset instead of resolving it against whatever
// directory the test binary happens to run in.
func envAbsPath(name string) string {
	p := strings.TrimSpace(os.Getenv(name))
	if !filepath.IsAbs(p) {
		return ""
	}
	return filepath.Clean(p)
}
