package theme

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// UserThemeError records why one user theme file could not be loaded.
// Name is a best-effort extraction of the file's declared top-level
// `name = "..."` (via peekDeclaredName), so a file that fails to parse
// AFTER a valid name line can still be matched by Resolve when that name
// is requested. Name is "" when even that could not be recovered (the
// name line itself is missing, malformed, or never reached).
type UserThemeError struct {
	Path string
	Name string
	Err  error
}

func (e UserThemeError) String() string {
	return fmt.Sprintf("%s: %v", e.Path, e.Err)
}

// DiscoverUserThemes scans dir (typically
// $XDG_CONFIG_HOME/deck/themes, i.e. filepath.Dir(config.toml)/themes)
// for *.toml files and parses each as a theme (§11.6, requirement 28's
// loader half).
//
// A missing or unreadable directory is not an error: most installations
// have no user themes at all, and start-up must proceed with the
// built-ins. Individual files that fail to parse are likewise not fatal
// to discovery — they are reported in the returned errs slice instead,
// so one broken user theme never prevents deck from starting or from
// loading every OTHER user theme.
//
// Files are read in filename order. If two files declare the same
// `name`, the alphabetically LATER filename wins (documented here as the
// duplicate-name rule for this loader; §11.6 does not specify one).
func DiscoverUserThemes(dir string) (themes map[string]*Theme, errs []UserThemeError) {
	themes = make(map[string]*Theme)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return themes, nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, fname := range names {
		path := filepath.Join(dir, fname)
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			errs = append(errs, UserThemeError{Path: path, Err: readErr})
			continue
		}
		t, parseErr := Parse(data, path)
		if parseErr != nil {
			errs = append(errs, UserThemeError{Path: path, Name: peekDeclaredName(data), Err: parseErr})
			continue
		}
		themes[t.Name] = t
	}
	return themes, errs
}

// peekDeclaredName best-effort extracts a theme file's top-level
// `name = "..."` line without requiring the rest of the file to parse
// cleanly, so DiscoverUserThemes can still associate a parse failure with
// the name an operator would have requested. It stops at the first
// section header, since §11.6 places `name` only at the top level, above
// [colors]. Returns "" if no valid name line is found before that point.
func peekDeclaredName(data []byte) string {
	for _, raw := range strings.Split(string(data), "\n") {
		text := strings.TrimSpace(stripComment(raw))
		if text == "" {
			continue
		}
		if strings.HasPrefix(text, "[") {
			return ""
		}
		key, value, err := parseKeyValue(text)
		if err != nil {
			continue
		}
		if key == "name" {
			s, err := unquoteString(value)
			if err != nil {
				return ""
			}
			return s
		}
	}
	return ""
}

// ThemesDir returns the user themes directory that sits alongside
// configFile (a deck config.toml path), i.e. filepath.Dir(configFile)
// + "/themes". This is $XDG_CONFIG_HOME/deck/themes under the normal XDG
// layout, and DECK_HOME/themes under the DECK_HOME override — the same
// directory config.toml itself lives in either way, so scenarios and
// installations that redirect one redirect the other identically.
func ThemesDir(configFile string) string {
	return filepath.Join(filepath.Dir(configFile), "themes")
}

// Resolve picks the active theme by declared name, given the themes
// DiscoverUserThemes found (plus the errors it reported) and the
// embedded built-ins, and reports a human-readable reason string
// whenever it had to fall back to the default (task 019 shows this
// string to the user on first paint; Resolve only computes it).
//
// name == "" means nothing was configured: the default is returned with
// no reason, since that is not a fallback from a request, it is the
// absence of one.
//
// Precedence when a user theme and a built-in declare the same name: the
// USER theme wins. A user theme file is how an operator overrides or
// restyles a distributed name; §11.6 defines no separate namespace for
// user vs. built-in names, so preferring the built-in would make an
// override of a built-in's name unreachable by that name. This is the
// "shadow/precedence rule" task 009 asks be chosen and documented.
//
// An unknown name (matches neither a user theme nor a built-in) falls
// back to the default with a reason naming the unknown name.
//
// A name that matches only a user theme file that failed to parse (via
// its peeked declared name — see peekDeclaredName) falls back to the
// default with a reason naming that specific FILE, not just the name,
// since the file is what an operator would need to go fix.
func Resolve(userThemes map[string]*Theme, userErrs []UserThemeError, name string) (*Theme, string) {
	def := Default()
	if name == "" {
		return def, ""
	}
	if t, ok := userThemes[name]; ok {
		return t, ""
	}
	if t, ok := Builtin(name); ok {
		return t, ""
	}
	for _, e := range userErrs {
		if e.Name == name {
			return def, fmt.Sprintf("theme %q: %s failed to parse (%v); using default theme %q", name, e.Path, e.Err, def.Name)
		}
	}
	return def, fmt.Sprintf("theme %q not found; using default theme %q", name, def.Name)
}
