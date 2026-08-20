package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// loadConfigFile reads config.toml's implemented top-level controls, the
// [ui] table and the [env] table. It intentionally does not
// attempt a general TOML parser — deck's config.toml also carries [notify]
// and [[notify.rule]] tables (SPEC §10) that are out of scope here, so
// unrecognised sections are skipped rather than misparsed. A missing file
// yields the documented defaults (allow_yolo=false, mouse=true, no env) and
// no error; a file that exists but cannot be understood yields a stated
// error, never a silent default.
func loadConfigFile(path string) (allowYolo bool, staleAfter time.Duration, mouse bool, env map[string]string, err error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, DefaultStaleAfter, true, nil, nil
		}
		return false, 0, false, nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	allowYolo = false
	staleAfter = DefaultStaleAfter
	mouse = true
	section := ""

	scanner := bufio.NewScanner(file)
	line := 0
	for scanner.Scan() {
		line++
		raw := scanner.Text()
		text := stripComment(raw)
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		if strings.HasPrefix(text, "[") {
			name, err := parseSectionHeader(text)
			if err != nil {
				return false, 0, false, nil, fmt.Errorf("%s:%d: %w", path, line, err)
			}
			section = name
			continue
		}
		key, value, err := parseKeyValue(text)
		if err != nil {
			return false, 0, false, nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		switch section {
		case "":
			switch key {
			case "allow_yolo":
				parsed, err := strconv.ParseBool(value)
				if err != nil {
					return false, 0, false, nil, fmt.Errorf("%s:%d: allow_yolo must be true or false, got %q", path, line, value)
				}
				allowYolo = parsed
			case "stale_after":
				var parsed time.Duration
				if strings.HasPrefix(value, "\"") {
					text, err := unquoteString(value)
					if err == nil {
						parsed, err = time.ParseDuration(text)
					}
					if err != nil {
						return false, 0, false, nil, fmt.Errorf("%s:%d: stale_after must be seconds or a duration, got %q", path, line, value)
					}
				} else {
					seconds, err := strconv.Atoi(value)
					if err != nil {
						return false, 0, false, nil, fmt.Errorf("%s:%d: stale_after must be seconds or a duration, got %q", path, line, value)
					}
					parsed = time.Duration(seconds) * time.Second
				}
				if parsed <= 0 {
					return false, 0, false, nil, fmt.Errorf("%s:%d: stale_after must be positive, got %q", path, line, value)
				}
				staleAfter = parsed
			}
			// Other top-level keys are ignored so future phases can add keys
			// without breaking this deliberately small parser.
		case "ui":
			switch key {
			case "mouse":
				parsed, err := strconv.ParseBool(value)
				if err != nil {
					return false, 0, false, nil, fmt.Errorf("%s:%d: mouse must be true or false, got %q", path, line, value)
				}
				mouse = parsed
			}
			// Other [ui] keys (e.g. a future sidebar_width default) are
			// ignored here for the same reason as unrecognised top-level keys.
		case "env":
			unquoted, err := unquoteString(value)
			if err != nil {
				return false, 0, false, nil, fmt.Errorf("%s:%d: [env] value for %q must be a quoted string: %w", path, line, key, err)
			}
			if env == nil {
				env = make(map[string]string)
			}
			env[key] = unquoted
		default:
			// A recognised-but-out-of-scope section (e.g. [notify]): its
			// body is intentionally not interpreted.
		}
	}
	if err := scanner.Err(); err != nil {
		return false, 0, false, nil, fmt.Errorf("read %s: %w", path, err)
	}
	return allowYolo, staleAfter, mouse, env, nil
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
