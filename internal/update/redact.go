package update

import (
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// redacted replaces every secret-bearing part of a URL in a message.
const redacted = "***"

// userinfoRE matches the user[:password]@ part of any scheme://… URL in free
// text, so an error from net/http, gh or a wrapped cause is covered too. It
// follows Go's url.Parse rather than a guess at "safe" characters: the
// userinfo runs to the LAST '@' of the authority (a password may hold '@'),
// may hold any of ' ( ) ! $ & * + , ; = ~ % and, in a URL that does not even
// parse, any other byte. Only whitespace, a double quote, angle brackets, a
// backslash and the authority's own terminators / ? # end it.
var userinfoRE = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)[^\s"<>\\/?#]*@`)

// queryValueRE matches a query-string value (a mirror may carry a token
// there). A value runs to the next '&', '#', whitespace, double quote or
// angle bracket, so an apostrophe or any other byte inside it is masked too.
var queryValueRE = regexp.MustCompile(`([?&][^=\s&"<>#]+=)[^&\s"<>#]*`)

// minKnownSecret is the shortest configured secret RedactKnown replaces
// wherever it appears. A shorter one is still masked in every URL position
// by RedactText; replacing it everywhere would rewrite ordinary words.
const minKnownSecret = 4

// RedactURL is one configured URL with its credentials removed: the user and
// password become "***" and every query value is masked. The scheme, host and
// path stay, so the message still says which server failed. Unlike free text,
// raw is known to be a single URL, so when it does not parse (a password
// holding '/', a space or a byte url.Parse refuses) everything up to its last
// '@' is treated as userinfo, and a URL with no "://" is masked the same way.
func RedactURL(raw string) string {
	if _, err := url.Parse(raw); err == nil && strings.Contains(raw, "://") {
		return RedactText(raw)
	}
	head, tail := splitQuery(raw)
	if at := strings.LastIndex(head, "@"); at >= 0 {
		start := 0
		if i := strings.Index(head, "://"); i >= 0 && i < at {
			start = i + len("://")
		}
		head = head[:start] + redacted + head[at:]
	}
	return RedactText(head + tail)
}

// splitQuery cuts raw before its first '?' or '#'.
func splitQuery(raw string) (head, tail string) {
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		return raw[:i], raw[i:]
	}
	return raw, ""
}

// RedactText masks the credentials of every URL found in s. Anything that
// prints a configured release URL, or an error that may quote one, passes
// through it: no secret reaches stderr, the UI or a log line.
func RedactText(s string) string {
	s = userinfoRE.ReplaceAllString(s, "${1}"+redacted+"@")
	return queryValueRE.ReplaceAllString(s, "${1}"+redacted)
}

// RedactKnown is RedactText plus a second, position-independent pass for the
// one URL deck was configured with: every secret it carries (the whole
// userinfo, the password, every query value, each raw and percent-decoded)
// is replaced wherever it appears in s, so a cause that re-quotes or
// re-encodes the URL is covered as well as one that prints it verbatim.
func RedactKnown(s, configured string) string {
	for _, secret := range knownSecrets(configured) {
		s = strings.ReplaceAll(s, secret, redacted)
	}
	return RedactText(s)
}

// knownSecrets are the credential strings in a configured URL, longest first
// so a whole userinfo is replaced before the password inside it.
func knownSecrets(raw string) []string {
	head, tail := splitQuery(raw)
	cands := userinfoSecrets(head)
	if u, err := url.Parse(raw); err == nil && u.User != nil {
		cands = append(cands, u.User.String())
		if pw, ok := u.User.Password(); ok {
			cands = append(cands, pw)
		}
	}
	return withDecoded(append(cands, queryValues(tail)...))
}

// userinfoSecrets is the userinfo (and the password inside it) of a URL's
// part before its query, read both ways: Go's (the authority ends at the
// first '/') and that of a URL that does not parse (up to the last '@').
func userinfoSecrets(head string) []string {
	if i := strings.Index(head, "://"); i >= 0 {
		head = head[i+len("://"):]
	}
	auth, _, _ := strings.Cut(head, "/")
	var out []string
	for _, s := range []string{auth, head} {
		at := strings.LastIndex(s, "@")
		if at < 0 {
			continue
		}
		out = append(out, s[:at])
		if _, pw, ok := strings.Cut(s[:at], ":"); ok {
			out = append(out, pw)
		}
	}
	return out
}

// queryValues is every value in tail's "?…" query, up to a '#'.
func queryValues(tail string) []string {
	q, ok := strings.CutPrefix(tail, "?")
	if !ok {
		return nil
	}
	q, _, _ = strings.Cut(q, "#")
	var out []string
	for _, kv := range strings.Split(q, "&") {
		if _, v, ok := strings.Cut(kv, "="); ok {
			out = append(out, v)
		}
	}
	return out
}

// withDecoded is cands plus each one's percent-decoded forms, deduplicated,
// without anything shorter than minKnownSecret, longest first.
func withDecoded(cands []string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		if len(s) >= minKnownSecret && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, c := range cands {
		add(c)
		if d, err := url.PathUnescape(c); err == nil {
			add(d)
		}
		if d, err := url.QueryUnescape(c); err == nil {
			add(d)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}

// redactedError is err with its message redacted; the cause stays reachable
// through Unwrap, so errors.Is/As (Refusal, ErrUnsupportedPlatform) still work.
type redactedError struct {
	err error
	msg string
}

func (e *redactedError) Error() string { return e.msg }
func (e *redactedError) Unwrap() error { return e.err }

// redactErr returns err unchanged when its message holds no credential of
// any URL nor of the configured one, else an error with the same cause and a
// redacted message.
func redactErr(err error, configured string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if clean := RedactKnown(msg, configured); clean != msg {
		return &redactedError{err: err, msg: clean}
	}
	return err
}
