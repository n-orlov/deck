package update

import "regexp"

// redacted replaces every secret-bearing part of a URL in a message.
const redacted = "***"

// userinfoRE matches the user[:password]@ part of any scheme://… URL in free
// text, so an error from net/http, gh or a wrapped cause is covered too.
var userinfoRE = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)[^/\s"'@]*@`)

// queryValueRE matches a query-string value (a mirror may carry a token there).
var queryValueRE = regexp.MustCompile(`([?&][^=\s&"'#]+=)[^&\s"'#]*`)

// RedactURL is raw with its credentials removed: the user and password become
// "***" and every query value is masked. The scheme, host and path stay, so
// the message still says which server failed.
func RedactURL(raw string) string { return RedactText(raw) }

// RedactText masks the credentials of every URL found in s. Anything that
// prints a configured release URL, or an error that may quote one, passes
// through it: no secret reaches stderr, the UI or a log line.
func RedactText(s string) string {
	s = userinfoRE.ReplaceAllString(s, "${1}"+redacted+"@")
	return queryValueRE.ReplaceAllString(s, "${1}"+redacted)
}

// redactedError is err with its message redacted; the cause stays reachable
// through Unwrap, so errors.Is/As (Refusal, ErrUnsupportedPlatform) still work.
type redactedError struct {
	err error
	msg string
}

func (e *redactedError) Error() string { return e.msg }
func (e *redactedError) Unwrap() error { return e.err }

// redactErr returns err unchanged when its message holds no credential, else
// an error with the same cause and a redacted message.
func redactErr(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if clean := RedactText(msg); clean != msg {
		return &redactedError{err: err, msg: clean}
	}
	return err
}
