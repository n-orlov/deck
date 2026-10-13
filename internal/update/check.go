package update

import (
	"context"
	"strconv"
	"strings"
)

// CheckResult is what `deck update --check` reports.
type CheckResult struct {
	Current string // the running build's version
	Latest  string // the latest stable release tag
	Newer   bool   // Latest is a newer stable release than Current
}

// Check resolves the latest release and compares it with o.Current. It makes
// one request and changes nothing on disk. A build that is not a release
// ("dev") is never reported as having a newer release: it cannot be updated
// in place, so the answer would be a promise deck cannot keep.
func Check(ctx context.Context, o Options) (CheckResult, error) {
	o = o.withDefaults()
	base := baseURL(o.BaseURL)
	if err := CheckURL(base); err != nil {
		return CheckResult{}, err
	}
	latest, err := LatestTag(ctx, o.Client, base)
	if err != nil {
		return CheckResult{}, err
	}
	return CheckResult{Current: o.Current, Latest: latest, Newer: IsNewer(latest, o.Current)}, nil
}

// IsNewer reports whether tag is a newer stable vX.Y.Z than current. Anything
// that is not a stable tag (a "dev" build) is never older than tag's release
// for this purpose: it returns false.
func IsNewer(tag, current string) bool {
	t, ok := parseStable(tag)
	if !ok {
		return false
	}
	c, ok := parseStable(current)
	if !ok {
		return false
	}
	for i := range t {
		if t[i] != c[i] {
			return t[i] > c[i]
		}
	}
	return false
}

func parseStable(tag string) ([3]int, bool) {
	var out [3]int
	if !stableTag.MatchString(tag) {
		return out, false
	}
	for i, part := range strings.Split(tag[1:], ".") {
		n, err := strconv.Atoi(part)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
