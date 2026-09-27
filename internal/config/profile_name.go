package config

import (
	"fmt"
	"strings"
)

// ValidateProfileName is the one validator SPEC §3.4 requires at every entry
// point that names a profile: the positional argument, DECK_PROFILE, _hook's
// pane environment, and the --profiles directory scan. It implements
// ^[a-z0-9][a-z0-9_-]{0,15}$ -- 1-16 characters, lowercase letters, digits,
// "-" and "_", starting with a letter or digit -- and "default" always
// passes. Every rejection names what was typed and the rule broken, per
// SPEC's own examples.
func ValidateProfileName(name string) error {
	// Rule: only a-z, A-Z, 0-9, "-" and "_" appear anywhere in the name.
	// Checked before case, so "acme.prod" is reported for its "." rather
	// than (incorrectly) for case.
	for _, r := range name {
		if isAllowedProfileNameRune(r) {
			continue
		}
		return fmt.Errorf(`error: profile name %q contains %q; allowed: a-z 0-9 - _`, name, string(r))
	}

	// Rule: lowercase only, so "Work" and "work" never collide on a
	// case-insensitive filesystem.
	if lower := strings.ToLower(name); lower != name {
		return fmt.Errorf(`error: profile names are lowercase; did you mean %q?`, lower)
	}

	// Rule: 1-16 characters.
	if n := len(name); n < 1 || n > 16 {
		return fmt.Errorf(`error: profile name %q is %d characters; the limit is 16`, name, n)
	}

	// Rule: the first character is a letter or digit -- excludes flags,
	// "_" internals, "." and "..".
	first := rune(name[0])
	if !isAlphaNumericProfileNameRune(first) {
		return fmt.Errorf(`error: profile name %q starts with %q; must start with a-z or 0-9`, name, string(first))
	}

	return nil
}

// ValidateProfileNameEnv validates a name that came from the DECK_PROFILE
// environment variable, prefixing any failure with `DECK_PROFILE="…": ` per
// SPEC §3.4. An empty value is unset and never reaches here as a failure;
// callers resolve that before calling this.
func ValidateProfileNameEnv(name string) error {
	if err := ValidateProfileName(name); err != nil {
		return fmt.Errorf(`DECK_PROFILE=%q: %w`, name, err)
	}
	return nil
}

func isAllowedProfileNameRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_'
}

func isAlphaNumericProfileNameRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}
