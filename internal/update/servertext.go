package update

import (
	"errors"
	"net/url"
	"reflect"
	"sort"
	"strings"
)

// serverText marks text deck did not write: what a release server sent back
// (a status reason phrase, a redirect path, a checksums.txt field, bytes a
// transport or body-read error quotes) or what gh printed. A server can echo
// the credential deck sent it there, in any position and at any length, so
// redactErr masks every configured secret wherever it stands inside such
// text, however short (see scrubServerText); deck's own wording around it is
// left alone, so its diagnostics stay readable.
type serverText struct {
	err  error  // a cause whose message is server text (nil for a bare string)
	text string // the server text itself, when err is nil
}

func (e *serverText) Error() string {
	if e.err != nil {
		return e.err.Error()
	}
	return e.text
}

func (e *serverText) Unwrap() error { return e.err }

// isServerText tells serverTexts a node of an error tree is server text
// itself rather than something wrapping it.
func (e *serverText) isServerText() {}

// fromServer marks err's whole message as server text; errors.Is/As still
// reach err through Unwrap.
func fromServer(err error) error {
	if err == nil {
		return nil
	}
	return &serverText{err: err}
}

// serverString marks s as server text.
func serverString(s string) error { return &serverText{text: s} }

// serverTexts is the message of every serverText in err's tree, and of every
// foreignError in it, longest first, without descending into one (its message
// already holds its cause's).
func serverTexts(err error) []string {
	var out []string
	var walk func(error)
	walk = func(e error) {
		if e == nil {
			return
		}
		if _, ok := e.(interface{ isServerText() }); ok || foreignError(e) {
			out = append(out, e.Error())
			return
		}
		if u, ok := e.(interface{ Unwrap() []error }); ok {
			for _, c := range u.Unwrap() {
				walk(c)
			}
			return
		}
		walk(errors.Unwrap(e))
	}
	walk(err)
	sort.SliceStable(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}

// scrubServerText masks every secret of the configured URL in server text s,
// whatever its length: first where a URL anchor pins it, then in every form
// (raw, percent-decoded, percent-encoded, Go-quoted) wherever else it
// appears. Only an occurrence lying wholly inside a non-secret part of the
// configured URL (its scheme, host, port, path or a query key) is kept, so a
// one-byte secret does not erase the host a dial error names. A cause may
// also quote only the start of what the server sent (url.EscapeError quotes
// "%" and the next two bytes), so a prefix of a secret, two bytes or longer,
// that a closing quote cuts off is masked too.
func scrubServerText(s, configured string) string {
	s = replaceAnchored(s, configured)
	keep := keptParts(configured)
	forms := serverSecretForms(configured)
	for _, form := range forms {
		s = replaceOutside(s, form, redacted, keep)
	}
	for _, form := range forms {
		for n := len(form) - 1; n >= 2; n-- {
			s = replaceOutside(s, form[:n]+`"`, redacted+`"`, keep)
		}
	}
	return s
}

// serverSecretForms is every form of every secret in the configured URL, of
// any length, longest first.
func serverSecretForms(configured string) []string {
	seen := map[string]bool{}
	var out []string
	for _, a := range anchoredSecrets(configured) {
		for _, f := range secretForms(a.secret) {
			if !seen[f] {
				seen[f] = true
				out = append(out, f)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}

// keptParts are the non-secret parts of the configured URL.
func keptParts(configured string) []string {
	u, err := url.Parse(configured)
	if err != nil {
		return nil
	}
	parts := []string{u.Scheme + "://", u.Host, u.Hostname(), u.Port(), u.Path}
	for _, kv := range queryPairs("?" + u.RawQuery) {
		parts = append(parts, "?"+kv[0]+"=", "&"+kv[0]+"=")
	}
	return parts
}

// replaceOutside replaces every occurrence of secret in s with repl unless
// it lies wholly inside an occurrence of one of keep.
func replaceOutside(s, secret, repl string, keep []string) string {
	if secret == "" || !strings.Contains(s, secret) {
		return s
	}
	kept := keptBytes(s, keep)
	var b strings.Builder
	i := 0
	for i < len(s) {
		j := strings.Index(s[i:], secret)
		if j < 0 {
			break
		}
		start, end := i+j, i+j+len(secret)
		if allTrue(kept[start:end]) {
			b.WriteString(s[i : start+1])
			i = start + 1
			continue
		}
		b.WriteString(s[i:start])
		b.WriteString(repl)
		i = end
	}
	b.WriteString(s[i:])
	return b.String()
}

// keptBytes marks every byte of s that lies inside an occurrence of one of
// keep.
func keptBytes(s string, keep []string) []bool {
	kept := make([]bool, len(s))
	for _, k := range keep {
		for i := 0; k != "" && i < len(s); {
			j := strings.Index(s[i:], k)
			if j < 0 {
				break
			}
			for n := i + j; n < i+j+len(k); n++ {
				kept[n] = true
			}
			i += j + 1
		}
	}
	return kept
}

func allTrue(bs []bool) bool {
	for _, b := range bs {
		if !b {
			return false
		}
	}
	return true
}

// deckModule is the module path of deck's own packages.
var deckModule = strings.TrimSuffix(reflect.TypeOf(serverText{}).PkgPath(), "/internal/update")

// localErrorPkgs declare errors about this machine only (a file, a process,
// a syscall, a deadline): their text never quotes what a server sent.
var localErrorPkgs = map[string]bool{
	"io/fs": true, "os": true, "os/exec": true, "syscall": true, "context": true, "internal/poll": true,
}

// foreignError reports whether e's concrete type is declared outside deck,
// fmt, errors and localErrorPkgs: a *url.Error, a net or TLS error, an
// x509 or url.EscapeError, a type deck has never seen. Such an error may
// quote what a server sent (a Location header, a status line, a host name),
// so it is server text even where no boundary marked it with fromServer: a
// boundary deck forgot to mark then costs readability of that cause, never a
// leaked credential.
func foreignError(e error) bool {
	t := reflect.TypeOf(e)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	p := t.PkgPath()
	switch {
	case p == "fmt", p == "errors", localErrorPkgs[p]:
		return false
	case p == deckModule, strings.HasPrefix(p, deckModule+"/"):
		return false
	}
	return true
}
