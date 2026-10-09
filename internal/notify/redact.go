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

// maskSecretAssignments masks the value of every secret-shaped KEY=VALUE or
// KEY: VALUE pair in text, where KEY is a run of [A-Za-z0-9_.-], VALUE is a
// quoted string or a run of non-space characters, and whitespace may surround
// the separator. It is a single linear scan over the separators, so a very
// long reason costs little more than reading it: a regular expression over the
// same text was ~50 ns a byte, which put the detached hook's session-end
// handoff past its bound under load (SPEC §10.3). The scan steps from
// separator to separator with strings functions rather than Go loops: see
// asciiSpace.
func maskSecretAssignments(text string) string {
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
