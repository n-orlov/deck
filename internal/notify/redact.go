package notify

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// Redact applies SPEC §6.4 to free text: every value of a secret-shaped
// KEY=VALUE pair in the text is masked, and every session env value is
// removed wherever it appears. The result never contains a session env
// value that the rules above cover.
func Redact(text string, sessionEnv map[string]string) string {
	return scrub(maskSecretAssignments(text), sessionEnv)
}

// The character classes of the free-text scan. asciiSpace is the regexp `\s`
// class (ASCII whitespace only); keyBytes is a KEY's character class. They
// are consumed by strings functions, never by a per-byte Go loop: under the
// nightly lane's -race -covermode=atomic every counted block is a
// race-detector atomic, and a loop step per byte of a megabyte reason ran
// the detached handoff past its bound (SPEC §10.3).
const (
	asciiSpace = " \t\n\f\r"
	keyBytes   = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_.-"
)

// maskKeyValuePairs masks the value of every secret-shaped KEY=VALUE or
// KEY: VALUE pair in text, where KEY is a run of [A-Za-z0-9_.-], VALUE is a
// quoted string or a run of non-space characters, and whitespace may surround
// the separator. It is a single linear scan over the separators, so a very
// long reason costs little more than reading it: a regular expression over the
// same text was ~50 ns a byte, which put the detached hook's session-end
// handoff past its bound under load (SPEC §10.3). The scan steps from
// separator to separator with strings functions rather than Go loops: see
// asciiSpace. findKeyValuePair is its key=value shape in maskSecretAssignments.
// It is held to the regular expression it replaced, walked so that a
// non-secret pair's value is searched too (redact_scan_test.go).
func maskKeyValuePairs(text string) string { return maskSpans(text, findKeyValuePair) }

// findKeyValuePair finds the value of the next secret-shaped KEY=VALUE or
// KEY: VALUE pair at or after from (see maskKeyValuePairs). The value of a
// pair whose key is not a secret is searched for pairs too, so the secret in
// "error: GITHUB_TOKEN=abc" is masked: the regular expression this scan
// replaced took GITHUB_TOKEN=abc for the value of error and left it whole.
func findKeyValuePair(text string, from int) (int, int, int, bool) {
	pos := from
	for pos < len(text) {
		sepAt := strings.IndexAny(text[pos:], "=:")
		if sepAt < 0 {
			return 0, 0, 0, false
		}
		sep := pos + sepAt
		// The pair around the separator: the key run before it (not reaching
		// back past pos, where the previous pair ended) and the offset where
		// its value starts. No key before it or no value after it: no pair.
		beforeSep := strings.TrimRight(text[pos:sep], asciiSpace)
		keyStart, keyEnd := pos+len(strings.TrimRight(beforeSep, keyBytes)), pos+len(beforeSep)
		valStart := len(text) - len(strings.TrimLeft(text[sep+1:], asciiSpace))
		if keyStart == keyEnd || valStart == len(text) {
			pos = sep + 1
			continue
		}
		if IsSecretShapedKey(text[keyStart:keyEnd]) {
			valEnd := assignmentValueEnd(text, valStart)
			return valStart, valEnd, valEnd, true
		}
		pos = valStart
	}
	return 0, 0, 0, false
}

// The other secret shapes maskSecretAssignments finds (SPEC §10.1). Every one
// is a span finder over strings functions, like the key=value scan: it
// returns the next span to mask and where to resume, and
// maskSecretAssignments merges the spans of every shape in one walk.
const (
	// tokenBytes is the body of a key, token or JWT segment.
	tokenBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-"
	// minPrefixedBody is the shortest body after a well-known prefix that is
	// taken for a key: shorter is an ordinary word such as "sk-learn".
	minPrefixedBody = 8
)

// wellKnownPrefixes are the prefixes whose following body is a credential by
// itself: an API key (sk-), a GitHub token (ghp_/gho_/ghu_/ghs_/ghr_,
// github_pat_) and an AWS access key id (AKIA).
var wellKnownPrefixes = []string{"sk-", "ghp_", "gho_", "ghu_", "ghs_", "ghr_", "github_pat_", "AKIA"}

// secretFlagWords are the last words of a flag name (--token, --api-key,
// --client_secret) that carry a secret as the flag's value. The whole last
// word must match, so --tokens-per-minute and --max-tokens are not secrets.
var secretFlagWords = map[string]bool{
	"token": true, "secret": true, "key": true, "password": true, "passwd": true, "credential": true,
}

// spanFinder returns the next span [start, end) of text at or after from that
// holds a secret, and where the search resumes. ok is false when there is no
// further span.
type spanFinder func(text string, from int) (start, end, next int, ok bool)

// maskSpans replaces every span find reports with the placeholder.
func maskSpans(text string, find spanFinder) string {
	var out strings.Builder
	copied, pos := 0, 0
	for pos < len(text) {
		start, end, next, ok := find(text, pos)
		if !ok {
			break
		}
		if out.Cap() == 0 {
			out.Grow(len(text))
		}
		out.WriteString(text[copied:start])
		out.WriteString(MaskedPlaceholder)
		copied = end
		pos = next
	}
	if copied == 0 {
		return text
	}
	out.WriteString(text[copied:])
	return out.String()
}

// maskSecretAssignments masks every secret shape in text (SPEC §10.1): the
// value of a KEY=VALUE or KEY: VALUE pair whose key is secret-shaped, also
// when the key is quoted ("api_key": "v"); the token after Bearer; the value
// of a secret-shaped flag (--token abc, --password=abc); a body behind a
// well-known prefix (sk-, ghp_, github_pat_, AKIA); and a JWT. None uses a
// regular expression (see maskKeyValuePairs).
//
// Every shape is found in the original text, and the spans are merged in one
// left-to-right walk: the earliest span wins and absorbs every span that
// overlaps it. Masking shape by shape instead let a later shape inside a
// quoted value (the Bearer in --password "a\" Bearer b") be masked first,
// taking the value's closing quote with it, so the quoted value's own mask
// stopped at a space and left its suffix in the clear.
func maskSecretAssignments(text string) string {
	finders := []spanFinder{findQuotedKeyValue, findBearerToken, findSecretFlagValue}
	for _, prefix := range wellKnownPrefixes {
		finders = append(finders, func(text string, from int) (int, int, int, bool) {
			return findPrefixedBody(text, from, prefix)
		})
	}
	finders = append(finders, findJWT, findKeyValuePair)
	merger := spanMerger{cursors: make([]spanCursor, len(finders))}
	for i, find := range finders {
		merger.cursors[i].find = find
	}
	return maskSpans(text, merger.find)
}

// spanCursor holds one shape's next span, so the merged walk asks each shape
// again only once the walk has passed that span: every shape's search stays
// one forward scan, and the merge stays linear.
type spanCursor struct {
	find              spanFinder
	start, end        int
	queried, ok, done bool
}

// advance finds the shape's next span at or after from, unless the one it
// holds already starts there or later.
func (c *spanCursor) advance(text string, from int) {
	if c.done || (c.queried && c.start >= from) {
		return
	}
	c.start, c.end, _, c.ok = c.find(text, from)
	c.queried, c.done = true, !c.ok
}

// spanMerger merges the spans of every shape into non-overlapping spans.
type spanMerger struct{ cursors []spanCursor }

// find is a spanFinder over every shape at once: the earliest span at or
// after from, grown by every span that starts inside it.
func (m *spanMerger) find(text string, from int) (int, int, int, bool) {
	start, ok := m.earliest(text, from)
	if !ok {
		return 0, 0, 0, false
	}
	end := m.absorb(text, start, start)
	return start, end, end, true
}

// earliest is where the first span at or after from starts.
func (m *spanMerger) earliest(text string, from int) (int, bool) {
	start, ok := 0, false
	for i := range m.cursors {
		c := &m.cursors[i]
		c.advance(text, from)
		if c.ok && (!ok || c.start < start) {
			start, ok = c.start, true
		}
	}
	return start, ok
}

// absorb grows the span [start, end) by every span that starts at start or
// inside it, until none does, and moves each absorbed shape past it.
func (m *spanMerger) absorb(text string, start, end int) int {
	for grown := true; grown; {
		grown = false
		for i := range m.cursors {
			c := &m.cursors[i]
			for c.ok && (c.start < end || c.start == start) {
				if c.end > end {
					end, grown = c.end, true
				}
				c.advance(text, c.end)
			}
		}
	}
	return end
}

// startsWord reports whether index i begins a word: it is not preceded by a
// key character, so "task-x" is not an "sk-" and "a.eyJ" is not a JWT.
func startsWord(text string, i int) bool {
	return i == 0 || strings.IndexByte(keyBytes, text[i-1]) < 0
}

// runLen is the length of the leading run of text made of bytes in set.
func runLen(text, set string) int { return len(text) - len(strings.TrimLeft(text, set)) }

// findQuotedKeyValue finds the value of a secret-shaped KEY whose key is
// quoted, "api_key": "v" or 'password'='v', which maskKeyValuePairs does not
// see because the quote sits between the key and the separator.
func findQuotedKeyValue(text string, from int) (int, int, int, bool) {
	for from < len(text) {
		sepAt := strings.IndexAny(text[from:], "=:")
		if sepAt < 0 {
			return 0, 0, 0, false
		}
		sep := from + sepAt
		if start, end, ok := quotedKeyValueAt(text, from, sep); ok {
			return start, end, end, true
		}
		from = sep + 1
	}
	return 0, 0, 0, false
}

// quotedKeyValueAt reports the value span behind the separator at sep when
// the text between from and sep ends in a quoted secret-shaped key.
func quotedKeyValueAt(text string, from, sep int) (int, int, bool) {
	before := strings.TrimRight(text[from:sep], asciiSpace)
	quoteAt := from + len(before) - 1
	valStart := len(text) - len(strings.TrimLeft(text[sep+1:], asciiSpace))
	if len(before) == 0 || valStart == len(text) || !isQuote(text[quoteAt]) {
		return 0, 0, false
	}
	keyStart := from + len(strings.TrimRight(before[:len(before)-1], keyBytes))
	if keyStart == quoteAt || keyStart == 0 || text[keyStart-1] != text[quoteAt] ||
		!IsSecretShapedKey(text[keyStart:quoteAt]) {
		return 0, 0, false
	}
	return valStart, assignmentValueEnd(text, valStart), true
}

// isQuote reports whether b opens or closes a quoted string.
func isQuote(b byte) bool { return b == '"' || b == '\'' }

// findBearerToken finds the token after the word Bearer (any case), as in an
// Authorization header or a curl argument: up to the next whitespace, or a
// quoted token through its closing quote.
func findBearerToken(text string, from int) (int, int, int, bool) {
	for from < len(text) {
		at := strings.IndexAny(text[from:], "bB")
		if at < 0 {
			return 0, 0, 0, false
		}
		i := from + at
		from = i + 1
		afterWord := i + len("bearer")
		if afterWord >= len(text) || !strings.EqualFold(text[i:afterWord], "bearer") || !startsWord(text, i) {
			continue
		}
		start := len(text) - len(strings.TrimLeft(text[afterWord:], asciiSpace))
		if start == afterWord || start == len(text) {
			continue
		}
		end := assignmentValueEnd(text, start)
		return start, end, end, true
	}
	return 0, 0, 0, false
}

// findSecretFlagValue finds the value of a secret-shaped command-line flag,
// --token abc, --api-key=abc, --client_secret "a b". The flag's last word
// decides (secretFlagWords); a following flag is not a value.
func findSecretFlagValue(text string, from int) (int, int, int, bool) {
	for from < len(text) {
		at := strings.Index(text[from:], "--")
		if at < 0 {
			return 0, 0, 0, false
		}
		start, end, next, ok := secretFlagValueAt(text, from+at)
		if ok {
			return start, end, end, true
		}
		from = next
	}
	return 0, 0, 0, false
}

// secretFlagValueAt reports the value span of the flag whose "--" sits at i,
// when the flag is secret-shaped and has a value, and where the search
// resumes either way.
func secretFlagValueAt(text string, i int) (start, end, next int, ok bool) {
	nameStart := i + 2
	nameLen := runLen(text[nameStart:], keyBytes)
	if nameLen == 0 || !startsWord(text, i) {
		return 0, 0, nameStart, false
	}
	next = nameStart + nameLen
	name := text[nameStart:next]
	if next >= len(text) || !secretFlagWords[strings.ToLower(name[strings.LastIndexAny(name, "-_.")+1:])] {
		return 0, 0, next, false
	}
	start, ok = flagValueStart(text, next)
	if !ok {
		return 0, 0, next, false
	}
	return start, assignmentValueEnd(text, start), next, true
}

// flagValueStart reports where a flag's value begins, given the index just
// past the flag's name: after an "=", else after the separating whitespace.
// A following flag is not a value.
func flagValueStart(text string, at int) (int, bool) {
	start := at + 1
	if text[at] != '=' {
		start = len(text) - len(strings.TrimLeft(text[at:], asciiSpace))
		if start == at {
			return 0, false
		}
	}
	if start >= len(text) || strings.HasPrefix(text[start:], "--") {
		return 0, false
	}
	return start, true
}

// findPrefixedBody finds a credential that announces itself by prefix: the
// prefix and the body after it, when the body is long enough to be a key.
func findPrefixedBody(text string, from int, prefix string) (int, int, int, bool) {
	for from < len(text) {
		at := strings.Index(text[from:], prefix)
		if at < 0 {
			return 0, 0, 0, false
		}
		i := from + at
		bodyStart := i + len(prefix)
		from = bodyStart
		if !startsWord(text, i) {
			continue
		}
		if body := runLen(text[bodyStart:], tokenBytes); body >= minPrefixedBody {
			return i, bodyStart + body, bodyStart + body, true
		}
	}
	return 0, 0, 0, false
}

// findJWT finds a three-segment JSON web token: a base64url header (which
// always begins eyJ, the encoding of {"), a payload and a signature, joined by
// dots.
func findJWT(text string, from int) (int, int, int, bool) {
	for from < len(text) {
		at := strings.Index(text[from:], "eyJ")
		if at < 0 {
			return 0, 0, 0, false
		}
		i := from + at
		from = i + 3
		if !startsWord(text, i) {
			continue
		}
		if end := jwtEnd(text, i); end > 0 {
			return i, end, end, true
		}
	}
	return 0, 0, 0, false
}

// jwtEnd returns where the token whose header starts at i ends, or -1 when
// the header is not followed by two dot-joined, non-empty segments.
func jwtEnd(text string, i int) int {
	end := i + runLen(text[i:], tokenBytes)
	for segment := 0; segment < 2; segment++ {
		if end >= len(text) || text[end] != '.' {
			return -1
		}
		n := runLen(text[end+1:], tokenBytes)
		if n == 0 {
			return -1
		}
		end += 1 + n
	}
	return end
}

// assignmentValueEnd returns where the value starting at start ends: after
// the closing quote of a quoted string, else at the next whitespace. A
// backslash escapes the byte after it inside a quoted string, so "a\"b" is one
// value and the secret is masked through its real closing quote, never to an
// escaped one that leaves a suffix in the clear. A quote that never closes
// falls back to the next whitespace, as the regular expression this scan
// replaced did.
func assignmentValueEnd(text string, start int) int {
	if quote := text[start]; isQuote(quote) {
		if closing := closingQuote(text, start+1, quote); closing >= 0 {
			return closing + 1
		}
	}
	if i := strings.IndexAny(text[start+1:], asciiSpace); i >= 0 {
		return start + 1 + i
	}
	return len(text)
}

// closingQuote returns the index of the first quote at or after from that is
// not escaped by a backslash, or -1. It jumps from quote or backslash to the
// next with strings.IndexAny, so it stays linear (see asciiSpace).
func closingQuote(text string, from int, quote byte) int {
	stops := `\` + string(quote)
	for from < len(text) {
		at := strings.IndexAny(text[from:], stops)
		if at < 0 {
			return -1
		}
		from += at
		if text[from] == quote {
			return from
		}
		from += 2 // a backslash and the byte it escapes
	}
	return -1
}

// scrubValues lists the session env values to remove by value, longest first
// so a value that contains another is removed whole. There is no length
// floor (SPEC §6.4: env values never appear): only the empty value, which
// has nothing to remove, is skipped.
func scrubValues(sessionEnv map[string]string) []string {
	values := make([]string, 0, len(sessionEnv))
	for _, value := range sessionEnv {
		if value == "" {
			continue
		}
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	return values
}

// truncate caps text at limit bytes on a rune boundary and marks the cut.
func truncate(text string, limit int) string {
	if limit <= 0 || len(text) <= limit {
		return text
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + "…"
}

// scrub removes every session env value from text by value, without the
// KEY=VALUE masking: it is for identity fields (a name, a path) where only
// the env values themselves must not survive.
func scrub(text string, sessionEnv map[string]string) string {
	for _, value := range scrubValues(sessionEnv) {
		text = strings.ReplaceAll(text, value, MaskedPlaceholder)
	}
	return text
}

// redactedError is an error whose text went through Redact, keeping the
// original in the chain for errors.Is/As.
type redactedError struct {
	text string
	err  error
}

func (e *redactedError) Error() string { return e.text }
func (e *redactedError) Unwrap() error { return e.err }

// redactError scrubs a recordable error so its text carries no session env
// value either (SPEC §10.1: none in the captured record).
func redactError(err error, sessionEnv map[string]string) error {
	return &redactedError{text: Redact(err.Error(), sessionEnv), err: err}
}

// stripNUL removes every NUL byte. A NUL cannot be carried in a process
// environment: exec refuses the whole spawn with "environment variable
// contains NUL", which would lose the event after its pair was claimed.
func stripNUL(text string) string {
	if !strings.ContainsRune(text, 0) {
		return text
	}
	return strings.ReplaceAll(text, "\x00", "")
}

// stripRequestNUL returns the request with NUL removed from every string that
// is exported in the environment or the payload: the session fields, the
// event's kind, reason and message, and the deck host and version (SPEC
// §10.1). It runs first, before the offered-kind check and the by-value
// scrub, so a value split by a NUL is still recognised. The configured
// command is left as written.
func stripRequestNUL(req Request) Request {
	s := &req.Session
	for _, field := range []*string{
		&s.ID, &s.Name, &s.Slug, &s.CWD, &s.Agent, &s.Group, &s.PermissionProfile,
		&s.ConversationID, &s.LaunchKind, &s.Status, &s.Reason,
		&req.Event.Kind, &req.Event.Reason, &req.Event.Message,
		&req.Deck.Host, &req.Deck.Version,
	} {
		*field = stripNUL(*field)
	}
	return req
}

// sanitize returns the request with every string that reaches the script's
// environment or payload scrubbed of session env values (SPEC §10.1: "Env
// values never appear" there). Free-text reasons also get the §6.4 masking;
// the message is handled by safeMessage, the kind is from the offered set.
func sanitize(req Request) Request {
	env := req.SessionEnv
	s := &req.Session
	for _, field := range []*string{
		&s.ID, &s.Name, &s.Slug, &s.CWD, &s.Agent, &s.Group, &s.PermissionProfile,
		&s.ConversationID, &s.LaunchKind, &s.Status, &req.Deck.Host, &req.Deck.Version,
	} {
		*field = scrub(*field, env)
	}
	s.Reason = Redact(s.Reason, env)
	req.Event.Reason = Redact(req.Event.Reason, env)
	return req
}
