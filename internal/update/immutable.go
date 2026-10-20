package update

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// EnvRequireImmutable controls whether a release that is not verified
// immutable may be installed. Requiring it is the default (fail closed): only
// the exact value "0" opts out, turning the refusal into a note. Any other
// value, an empty one included, keeps it required.
const EnvRequireImmutable = "DECK_REQUIRE_IMMUTABLE"

// immutableOptOut is the one EnvRequireImmutable value that opts out.
const immutableOptOut = "0"

// DefaultAPIBase is the GitHub REST API the release's immutability is read
// from. Only the default release base has it: a DECK_UPDATE_URL mirror has no
// API, so its releases are always ImmutableUnknown.
const DefaultAPIBase = "https://api.github.com"

// Immutability outcomes reported in Result.Immutable.
const (
	// ImmutableVerified: the release API answered `"immutable": true`.
	ImmutableVerified = "immutable"
	// ImmutableNot: the release API answered `"immutable": false`.
	ImmutableNot = "not-immutable"
	// ImmutableUnknown: the answer could not be had or read (a mirror, no
	// network, a rate limit, a non-200, an oversize or malformed body, a
	// missing or non-boolean field).
	ImmutableUnknown = "unknown"
)

// Immutability plans, the PlanImmutable answers.
const (
	// ImmutableReported: DECK_REQUIRE_IMMUTABLE=0 opted out; the
	// immutability is looked up and reported, and a release not verified
	// immutable still installs.
	ImmutableReported = "reported"
	// ImmutableRequired: the default (or Options.RequireImmutable); only a
	// release the API reports immutable installs.
	ImmutableRequired = "required"
	// ImmutableRefused: immutability is required but the base is a mirror
	// with no API, so the update would be refused whatever the release is.
	ImmutableRefused = "refused"
)

// maxReleaseAPIBytes caps the release API answer. A deck release object with
// its handful of assets is a few KB.
const maxReleaseAPIBytes = 1 << 20

// immutableTimeout bounds the release API request; tests shorten it.
var immutableTimeout = 15 * time.Second

// requireImmutable reports whether a release that is not verified immutable
// is an abort: always, unless DECK_REQUIRE_IMMUTABLE is exactly "0" and
// Options.RequireImmutable is not set.
func (o Options) requireImmutable() bool {
	return o.RequireImmutable || os.Getenv(EnvRequireImmutable) != immutableOptOut
}

// releaseAPIBase is the API base the release's immutability is read from, or
// "" when there is none (a mirror base).
func (o Options) releaseAPIBase(base string) string {
	if o.apiBase != "" {
		return o.apiBase
	}
	if base == DefaultBaseURL {
		return DefaultAPIBase
	}
	return ""
}

// PlanImmutable says, before anything is downloaded and without a request,
// what Run will do about the release's immutability: ImmutableReported,
// ImmutableRequired, or ImmutableRefused when it is required and the base has
// no release API to ask.
func PlanImmutable(o Options) string {
	switch {
	case !o.requireImmutable():
		return ImmutableReported
	case o.releaseAPIBase(baseURL(o.BaseURL)) == "":
		return ImmutableRefused
	}
	return ImmutableRequired
}

// releaseImmutability asks the GitHub release API whether tag is an
// immutable release of DefaultRepo (the repository is pinned). It
// never fails: anything short of a well-formed answer is ImmutableUnknown,
// and the cause is returned beside it for a refusal to quote.
func (o Options) releaseImmutability(ctx context.Context, base, tag string) (string, error) {
	api := o.releaseAPIBase(base)
	if api == "" {
		return ImmutableUnknown, fmt.Errorf("the release base %s has no GitHub release API", RedactURL(base))
	}
	ctx, cancel := context.WithTimeout(ctx, immutableTimeout)
	defer cancel()
	body, err := get(ctx, o.Client, api+"/repos/"+DefaultRepo+"/releases/tags/"+tag, maxReleaseAPIBytes)
	if err != nil {
		return ImmutableUnknown, err
	}
	return parseImmutable(body)
}

// parseImmutable reads the release object's "immutable" member strictly: only
// the JSON literal true is ImmutableVerified and only false is ImmutableNot.
// A body that is not a JSON object, or a member that is absent or of another
// type (a string "true", a number, null), is ImmutableUnknown.
func parseImmutable(body []byte) (string, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil || obj == nil {
		return ImmutableUnknown, fmt.Errorf("the release API answer is not a JSON object")
	}
	raw, ok := obj["immutable"]
	if !ok {
		return ImmutableUnknown, fmt.Errorf("the release API answer has no immutable field")
	}
	switch {
	case bytes.Equal(raw, []byte("true")):
		return ImmutableVerified, nil
	case bytes.Equal(raw, []byte("false")):
		return ImmutableNot, nil
	}
	return ImmutableUnknown, fmt.Errorf("the release API's immutable field is not a boolean")
}

// checkImmutable looks the release up and returns its immutability, or a
// RefusedNotImmutable Refusal when it is not verified immutable and
// immutability is required (the default). The refusal is deck's own wording,
// chosen from the state; no server text reaches it.
func (o Options) checkImmutable(ctx context.Context, base, tag string) (string, error) {
	state, _ := o.releaseImmutability(ctx, base, tag)
	if state == ImmutableVerified || !o.requireImmutable() {
		return state, nil
	}
	return "", refuse(RefusedNotImmutable, "release %s %s, and an immutable release is required; nothing was installed (set %s=%s to install it anyway)",
		tag, notImmutableCause(state, o.releaseAPIBase(base) == ""), EnvRequireImmutable, immutableOptOut)
}

// notImmutableCause says why a release is not verified immutable: GitHub
// said so, the release source is a mirror with no release API, or the API
// gave no usable answer.
func notImmutableCause(state string, mirror bool) string {
	switch {
	case state == ImmutableNot:
		return "is not a GitHub immutable release"
	case mirror:
		return "could not be confirmed immutable: the release source is a mirror with no GitHub release API"
	}
	return "could not be confirmed immutable: the GitHub release API gave no usable answer (offline, rate-limited or an unexpected reply)"
}
