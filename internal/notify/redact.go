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
// asciiSpace. It is the key=value pass of maskSecretAssignments and is held to
// the regular expression it replaced (redact_scan_test.go).
func maskKeyValuePairs(text string) string {
	var out strings.Builder
	copied, pos := 0, 0
	for pos < len(text) {
		sepAt := strings.IndexAny(text[pos:], "=:")
		if sepAt < 0 {
			break
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
		valEnd := assignmentValueEnd(text, valStart)
		if IsSecretShapedKey(text[keyStart:keyEnd]) {
			if out.Cap() == 0 {
				out.Grow(len(text))
			}
			out.WriteString(text[copied:valStart])
			out.WriteString(MaskedPlaceholder)
			copied = valEnd
		}
		pos = valEnd
	}
	if copied == 0 {
		return text
	}
	out.WriteString(text[copied:])
	return out.String()
}

// The other secret shapes maskSecretAssignments finds (SPEC §10.1). Every one
// is a separate linear pass over strings functions, like the key=value scan,
// that masks a span of the text: a span finder returns the span to mask and
// where to resume.
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
// well-known prefix (sk-, ghp_, github_pat_, AKIA); and a JWT. Each is its
// own linear pass: none uses a regular expression (see maskKeyValuePairs).
func maskSecretAssignments(text string) string {
	text = maskSpans(text, findQuotedKeyValue)
	text = maskSpans(text, findBearerToken)
	text = maskSpans(text, findSecretFlagValue)
	for _, prefix := range wellKnownPrefixes {
		text = maskSpans(text, func(text string, from int) (int, int, int, bool) {
			return findPrefixedBody(text, from, prefix)
		})
	}
	text = maskSpans(text, findJWT)
	return maskKeyValuePairs(text)
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
		before := strings.TrimRight(text[from:sep], asciiSpace)
		quoteAt := from + len(before) - 1
		valStart := len(text) - len(strings.TrimLeft(text[sep+1:], asciiSpace))
		if len(before) == 0 || valStart == len(text) || (text[quoteAt] != '"' && text[quoteAt] != '\'') {
			from = sep + 1
			continue
		}
		keyStart := from + len(strings.TrimRight(before[:len(before)-1], keyBytes))
		valEnd := assignmentValueEnd(text, valStart)
		if keyStart == quoteAt || keyStart == 0 || text[keyStart-1] != text[quoteAt] ||
			!IsSecretShapedKey(text[keyStart:quoteAt]) {
			from = sep + 1
			continue
		}
		return valStart, valEnd, valEnd, true
	}
	return 0, 0, 0, false
}

// findBearerToken finds the token after the word Bearer (any case), as in an
// Authorization header or a curl argument.
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
		end := len(text)
		if j := strings.IndexAny(text[start:], asciiSpace); j >= 0 {
			end = start + j
		}
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
		i := from + at
		nameStart := i + 2
		from = nameStart
		nameLen := runLen(text[nameStart:], keyBytes)
		if nameLen == 0 || !startsWord(text, i) {
			continue
		}
		name := text[nameStart : nameStart+nameLen]
		from = nameStart + nameLen
		if !secretFlagWords[strings.ToLower(name[strings.LastIndexAny(name, "-_.")+1:])] || from >= len(text) {
			continue
		}
		start := from + 1
		if text[from] != '=' {
			start = len(text) - len(strings.TrimLeft(text[from:], asciiSpace))
			if start == from {
				continue
			}
		}
		if start >= len(text) || strings.HasPrefix(text[start:], "--") {
			continue
		}
		end := assignmentValueEnd(text, start)
		return start, end, end, true
	}
	return 0, 0, 0, false
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
		end := i + runLen(text[i:], tokenBytes)
		for segment := 0; segment < 2 && end >= 0; segment++ {
			n := 0
			if end < len(text) && text[end] == '.' {
				n = runLen(text[end+1:], tokenBytes)
			}
			if n == 0 {
				end = -1
			} else {
				end += 1 + n
			}
		}
		if end > 0 {
			return i, end, end, true
		}
	}
	return 0, 0, 0, false
}

// assignmentValueEnd returns where the value starting at start ends: after
// the closing quote of a quoted string, else at the next whitespace.
func assignmentValueEnd(text string, start int) int {
	if quote := text[start]; quote == '"' || quote == '\'' {
		if closing := strings.IndexByte(text[start+1:], quote); closing >= 0 {
			return start + 1 + closing + 1
		}
	}
	if i := strings.IndexAny(text[start+1:], asciiSpace); i >= 0 {
		return start + 1 + i
	}
	return len(text)
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
