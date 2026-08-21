// Package theme implements deck's §11.6 theme system: a semantic, fixed
// token set that names meanings rather than widgets, a hand-rolled TOML
// loader (deck deliberately does not depend on a general TOML library —
// see internal/config/toml.go for the same choice), and two embedded
// built-in themes (one dark, one light).
package theme

// Token names a single semantic colour slot. The set is exactly §11.6's
// list: nothing invented, nothing omitted. A theme file that is missing
// one or names an extra key is rejected — see Parse.
type Token string

const (
	Background    Token = "background"
	Surface       Token = "surface"
	Border        Token = "border"
	BorderFocus   Token = "border_focus"
	Selection     Token = "selection"
	SelectionIdle Token = "selection_idle"
	Title         Token = "title"
	Text          Token = "text"
	Dimmed        Token = "dimmed"
	Hint          Token = "hint"
	Key           Token = "key"
	Accent        Token = "accent"
	Group         Token = "group"
	SearchMatch   Token = "search_match"
	Badge         Token = "badge"
	BadgeWarn     Token = "badge_warn"

	// The seven §7 status tokens. If §7 grows a status, this list grows a
	// token — see StatusTokens below, which is what task 014 iterates to
	// keep status->token mapping exhaustive.
	Waiting  Token = "waiting"
	Running  Token = "running"
	Idle     Token = "idle"
	Starting Token = "starting"
	Stopped  Token = "stopped"
	Error    Token = "error"
	Archived Token = "archived"
)

// AllTokens is §11.6's complete, ordered token set. Order matches the
// SPEC's [colors] table for readability in error messages and tests; it
// carries no other significance.
var AllTokens = []Token{
	Background,
	Surface,
	Border,
	BorderFocus,
	Selection,
	SelectionIdle,
	Title,
	Text,
	Dimmed,
	Hint,
	Key,
	Accent,
	Group,
	SearchMatch,
	Badge,
	BadgeWarn,
	Waiting,
	Running,
	Idle,
	Starting,
	Stopped,
	Error,
	Archived,
}

// StatusTokens is the seven §7 statuses, in the same order §7's status
// table lists them. A test pins this list equal to §7's seven statuses so
// the two can never drift silently.
var StatusTokens = []Token{
	Starting,
	Running,
	Waiting,
	Idle,
	Error,
	Stopped,
	Archived,
}

func isKnownToken(t Token) bool {
	for _, k := range AllTokens {
		if k == t {
			return true
		}
	}
	return false
}
