package notify

import (
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// assignment finds KEY=VALUE and KEY: VALUE pairs in free text, where VALUE
// is a quoted string or a run of non-space characters.
var assignment = regexp.MustCompile(`([A-Za-z0-9_.\-]+)(\s*[=:]\s*)("[^"]*"|'[^']*'|\S+)`)

// Redact applies SPEC §6.4 to free text: every value of a secret-shaped
// KEY=VALUE pair in the text is masked, and every session env value is
// removed wherever it appears. The result never contains a session env
// value that the rules above cover.
func Redact(text string, sessionEnv map[string]string) string {
	return scrub(assignment.ReplaceAllStringFunc(text, maskAssignment), sessionEnv)
}

// maskAssignment masks the value half of one matched KEY=VALUE pair when its
// key is secret-shaped and leaves every other pair alone.
func maskAssignment(match string) string {
	parts := assignment.FindStringSubmatch(match)
	if !IsSecretShapedKey(parts[1]) {
		return match
	}
	return parts[1] + parts[2] + MaskedPlaceholder
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
