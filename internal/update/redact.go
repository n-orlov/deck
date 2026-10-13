package update

import (
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// redacted replaces every secret-bearing part of a URL in a message.
const redacted = "***"

// userinfoRE matches the user[:password]@ part of any scheme://… URL in free
// text, so an error from net/http, gh or a wrapped cause is covered too. It
// follows Go's url.Parse rather than a guess at "safe" characters: the
// userinfo runs to the LAST '@' of the authority (a password may hold '@'),
// may hold any of ' ( ) ! $ & * + , ; = ~ % and, in a URL that does not even
// parse, any other byte. Only whitespace, a double quote, angle brackets and
// the authority's own terminators / ? # end it. A backslash escape (a
// Go-quoted URL writes '"' as \" and '\' as \\) is part of it, not an end.
var userinfoRE = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)(?:\\.|[^\s"<>\\/?#])*@`)

// queryValueRE matches a query-string value (a mirror may carry a token
// there). A value runs to the next '&', '#', whitespace, unescaped double
// quote or angle bracket, so an apostrophe, a backslash escape (\" inside a
// Go-quoted URL) or any other byte inside it is masked too.
var queryValueRE = regexp.MustCompile(`([?&][^=\s&"<>#]+=)(?:\\.|[^&\s"<>#\\])*`)

// userinfoStops and queryStops are the bytes that end a userinfo and a query
// value for the patterns above. A configured secret holding one of them is
// also replaced piece by piece (see fragments).
const (
	userinfoStops = " \t\r\n\"<>\\/?#"
	queryStops    = " \t\r\n\"<>\\&#"
)

// minKnownSecret is the shortest configured secret RedactKnown replaces
// wherever it appears. A shorter one is still masked in every URL position
// by RedactText; replacing it everywhere would rewrite ordinary words.
const minKnownSecret = 4

// RedactURL is one configured URL with its credentials removed: the user and
// password become "***" and every query value is masked. The scheme, host and
// path stay, so the message still says which server failed. Unlike free text,
// raw is known to be a single URL, so its own secrets are replaced whole
// first (a byte that would end a free-text match, such as '"', cannot cut a
// secret in two), and when it does not parse (a password holding '/', a space
// or a byte url.Parse refuses) everything up to its last '@' is treated as
// userinfo; a URL with no "://" is masked the same way.
func RedactURL(raw string) string {
	parses := false
	if _, err := url.Parse(raw); err == nil && strings.Contains(raw, "://") {
		parses = true
	}
	s := replaceKnown(raw, raw)
	if parses {
		return RedactText(s)
	}
	head, tail := splitQuery(s)
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

// RedactKnown is RedactText plus a first, position-independent pass for the
// one URL deck was configured with: every secret it carries (the whole
// userinfo, the password, every query value) is replaced wherever it appears
// in s, before any pattern runs, so a cause that re-quotes or re-encodes the
// URL is covered as well as one that prints it verbatim. Each secret is
// replaced raw, percent-decoded, percent-encoded and Go-quoted (\" for '"'),
// and then piece by piece, split at the bytes that end a pattern match, so
// even text an earlier, partial redaction already cut leaves no piece behind.
func RedactKnown(s, configured string) string {
	return RedactText(replaceKnown(s, configured))
}

// replaceKnown replaces every knownSecrets(configured) in s with "***".
func replaceKnown(s, configured string) string {
	s = replaceAnchored(s, configured)
	for _, secret := range knownSecrets(configured) {
		s = strings.ReplaceAll(s, secret, redacted)
	}
	return s
}

// anchoredSecret is a credential of any length together with the bytes that
// pin its position in a URL: a query value follows "key=", a userinfo follows
// "://" and ends at "@", a password follows ':' and ends at '@'.
type anchoredSecret struct{ before, secret, after string }

// replaceAnchored masks every anchoredSecrets(configured) in s, whatever its
// length: the anchor, not the secret's own bytes, says where it is, so a
// one-byte value or one holding a byte that ends a pattern match (K>Z, K Z) is
// masked whole and ordinary words are never rewritten.
func replaceAnchored(s, configured string) string {
	for _, a := range anchoredSecrets(configured) {
		for _, form := range secretForms(a.secret) {
			s = strings.ReplaceAll(s, a.before+form+a.after, a.before+redacted+a.after)
		}
	}
	return s
}

// anchoredSecrets lists the userinfo, password and query values of a
// configured URL with their anchors, longest secret first. A query value ends
// at '&', '#', a space, a quote or the end of the URL; "after" for those is
// empty, so only the value's leading anchor and its own bytes must match.
func anchoredSecrets(raw string) []anchoredSecret {
	head, tail := splitQuery(raw)
	var out []anchoredSecret
	for _, u := range userinfoSecrets(head) {
		if u != "" {
			out = append(out, anchoredSecret{"://", u, "@"}, anchoredSecret{":", u, "@"})
		}
	}
	for _, kv := range queryPairs(tail) {
		if kv[1] != "" {
			out = append(out, anchoredSecret{"?" + kv[0] + "=", kv[1], ""}, anchoredSecret{"&" + kv[0] + "=", kv[1], ""})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return len(out[i].secret) > len(out[j].secret) })
	return out
}

// secretForms is s raw, percent-decoded, percent-encoded and Go-quoted.
func secretForms(s string) []string {
	seen := map[string]bool{}
	var out []string
	for _, form := range []string{s, pathUnescaped(s), queryUnescaped(s)} {
		for _, f := range []string{form, url.PathEscape(form), url.QueryEscape(form), goQuoted(form)} {
			if f != "" && !seen[f] {
				seen[f] = true
				out = append(out, f)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}

// queryPairs is every key=value pair of tail's "?…" query, up to a '#'.
func queryPairs(tail string) [][2]string {
	q, ok := strings.CutPrefix(tail, "?")
	if !ok {
		return nil
	}
	q, _, _ = strings.Cut(q, "#")
	var out [][2]string
	for _, kv := range strings.Split(q, "&") {
		if k, v, ok := strings.Cut(kv, "="); ok {
			out = append(out, [2]string{k, v})
		}
	}
	return out
}

// knownSecrets are the credential strings in a configured URL, longest first
// so a whole userinfo is replaced before the password inside it, and a whole
// secret before its pieces.
func knownSecrets(raw string) []string {
	head, tail := splitQuery(raw)
	user := userinfoSecrets(head)
	if u, err := url.Parse(raw); err == nil && u.User != nil {
		user = append(user, u.User.String())
		if pw, ok := u.User.Password(); ok {
			user = append(user, pw)
		}
	}
	query := queryValues(tail)
	cands := append(append([]string{}, user...), query...)
	cands = append(cands, fragments(user, userinfoStops)...)
	cands = append(cands, fragments(query, queryStops)...)
	return withVariants(cands)
}

// fragments is every piece of every secret (and of its decoded forms) cut at
// the bytes in stops, so a piece a pattern match left behind is still known.
func fragments(secrets []string, stops string) []string {
	var out []string
	for _, s := range secrets {
		for _, form := range []string{s, pathUnescaped(s), queryUnescaped(s)} {
			out = append(out, strings.FieldsFunc(form, func(r rune) bool { return strings.ContainsRune(stops, r) })...)
		}
	}
	return out
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
	var out []string
	for _, kv := range queryPairs(tail) {
		out = append(out, kv[1])
	}
	return out
}

// withVariants is cands plus each one's percent-decoded, percent-encoded and
// Go-quoted forms, deduplicated, without anything shorter than
// minKnownSecret, longest first.
func withVariants(cands []string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		if len(s) >= minKnownSecret && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, c := range cands {
		for _, form := range []string{c, pathUnescaped(c), queryUnescaped(c)} {
			add(form)
			add(url.PathEscape(form))
			add(url.QueryEscape(form))
			add(goQuoted(form))
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}

// pathUnescaped and queryUnescaped are s percent-decoded, or s itself when it
// does not decode.
func pathUnescaped(s string) string {
	if d, err := url.PathUnescape(s); err == nil {
		return d
	}
	return s
}

func queryUnescaped(s string) string {
	if d, err := url.QueryUnescape(s); err == nil {
		return d
	}
	return s
}

// goQuoted is s as %q writes it inside the quotes (the form a net/http
// *url.Error prints a URL in).
func goQuoted(s string) string {
	q := strconv.Quote(s)
	return q[1 : len(q)-1]
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
// redacted message. Server text inside it (see serverText) is scrubbed of
// the configured secrets at any length first.
func redactErr(err error, configured string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	clean := msg
	for _, t := range serverTexts(err) {
		if scrubbed := scrubServerText(t, configured); scrubbed != t {
			clean = strings.ReplaceAll(clean, t, scrubbed)
		}
	}
	if clean = RedactKnown(clean, configured); clean != msg {
		return &redactedError{err: err, msg: clean}
	}
	return err
}
