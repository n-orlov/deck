package theme

import (
	"bufio"
	"fmt"
	"strings"
)

// Parse reads one theme TOML document (the shape in §11.6: top-level
// name/appearance strings plus a [colors] table of quoted hex strings)
// and returns a validated Theme. Like internal/config/toml.go, this is a
// small hand-rolled parser rather than a general TOML library — themes
// have exactly one shape, and a general parser would accept forms §11.6
// never defines.
//
// source names the origin (a file path, or "<builtin:name>") for error
// messages.
func Parse(data []byte, source string) (*Theme, error) {
	t := &Theme{Colors: make(map[Token]string)}

	section := ""
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	line := 0
	for scanner.Scan() {
		line++
		raw := scanner.Text()
		text := stripComment(raw)
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		var err error
		section, err = parseThemeLine(t, section, text)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", source, line, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", source, err)
	}
	if t.Name == "" {
		return nil, fmt.Errorf("%s: missing top-level name", source)
	}
	if err := t.Validate(); err != nil {
		return nil, err
	}
	quantized, err := quantizeColors(t.Colors)
	if err != nil {
		return nil, fmt.Errorf("%s: quantizing to reference palette: %w", source, err)
	}
	t.Quantized = quantized
	return t, nil
}

// parseThemeLine applies one comment-stripped, non-empty line to t and
// returns the section the next line is read in. Its errors carry no
// source:line prefix; Parse adds it.
func parseThemeLine(t *Theme, section, text string) (string, error) {
	if strings.HasPrefix(text, "[") {
		return parseThemeSection(text)
	}
	key, value, err := parseKeyValue(text)
	if err != nil {
		return section, err
	}
	return section, applyThemeKey(t, section, key, value)
}

func parseThemeSection(text string) (string, error) {
	name, err := parseSectionHeader(text)
	if err != nil {
		return "", err
	}
	if name != "colors" {
		return "", fmt.Errorf("unknown section %q (only [colors] is defined)", name)
	}
	return name, nil
}

func applyThemeKey(t *Theme, section, key, value string) error {
	switch section {
	case "":
		return applyTopLevelKey(t, key, value)
	case "colors":
		return applyColourKey(t, key, value)
	default:
		return fmt.Errorf("unknown section %q", section)
	}
}

func applyTopLevelKey(t *Theme, key, value string) error {
	switch key {
	case "name":
		s, err := unquoteString(value)
		if err != nil {
			return fmt.Errorf("name must be a quoted string: %w", err)
		}
		t.Name = s
	case "appearance":
		s, err := unquoteString(value)
		if err != nil {
			return fmt.Errorf("appearance must be a quoted string: %w", err)
		}
		t.Appearance = s
	default:
		return fmt.Errorf("unknown top-level key %q", key)
	}
	return nil
}

func applyColourKey(t *Theme, key, value string) error {
	tok := Token(key)
	if !isKnownToken(tok) {
		return fmt.Errorf("unknown colour token %q", key)
	}
	hex, err := unquoteString(value)
	if err != nil {
		return fmt.Errorf("%s must be a quoted hex string: %w", key, err)
	}
	hex, err = normalizeHex(hex)
	if err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	t.Colors[tok] = hex
	return nil
}

func normalizeHex(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if !strings.HasPrefix(s, "#") || len(s) != 7 {
		return "", fmt.Errorf("must be a #rrggbb hex colour, got %q", s)
	}
	for _, c := range s[1:] {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return "", fmt.Errorf("must be a #rrggbb hex colour, got %q", s)
		}
	}
	return s, nil
}

func stripComment(line string) string {
	inQuote := false
	for i, r := range line {
		switch r {
		case '"':
			inQuote = !inQuote
		case '#':
			if !inQuote {
				return line[:i]
			}
		}
	}
	return line
}

func parseSectionHeader(text string) (string, error) {
	if !strings.HasSuffix(text, "]") {
		return "", fmt.Errorf("malformed section header %q", text)
	}
	name := strings.TrimSpace(text[1 : len(text)-1])
	if name == "" {
		return "", fmt.Errorf("empty section header")
	}
	return name, nil
}

func parseKeyValue(text string) (key, value string, err error) {
	idx := strings.Index(text, "=")
	if idx < 0 {
		return "", "", fmt.Errorf("expected key = value, got %q", text)
	}
	key = strings.TrimSpace(text[:idx])
	value = strings.TrimSpace(text[idx+1:])
	if key == "" {
		return "", "", fmt.Errorf("empty key in %q", text)
	}
	if value == "" {
		return "", "", fmt.Errorf("empty value for key %q", key)
	}
	return key, value, nil
}

func unquoteString(value string) (string, error) {
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return "", fmt.Errorf("not a quoted string: %q", value)
	}
	return value[1 : len(value)-1], nil
}
