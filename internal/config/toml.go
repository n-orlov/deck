package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// loadConfigFile reads config.toml's documented Phase 1 surface: the
// top-level allow_yolo key and the [env] table. It intentionally does not
// attempt a general TOML parser — deck's config.toml also carries [notify]
// and [[notify.rule]] tables (SPEC \u00a710) that are out of scope here, so
// unrecognised sections are skipped rather than misparsed. A missing file
// yields the documented defaults (allow_yolo=false, no env) and no error; a
// file that exists but cannot be understood yields a stated error, never a
// silent default.
func loadConfigFile(path string) (bool, map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil, nil
		}
		return false, nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	allowYolo := false
	var env map[string]string
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
				return false, nil, fmt.Errorf("%s:%d: %w", path, line, err)
			}
			section = name
			continue
		}
		key, value, err := parseKeyValue(text)
		if err != nil {
			return false, nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		switch section {
		case "":
			if key == "allow_yolo" {
				parsed, err := strconv.ParseBool(value)
				if err != nil {
					return false, nil, fmt.Errorf("%s:%d: allow_yolo must be true or false, got %q", path, line, value)
				}
				allowYolo = parsed
			}
			// Other top-level keys are not part of Phase 1's documented
			// surface; they are ignored rather than rejected so future
			// phases can add keys without breaking this parser.
		case "env":
			unquoted, err := unquoteString(value)
			if err != nil {
				return false, nil, fmt.Errorf("%s:%d: [env] value for %q must be a quoted string: %w", path, line, key, err)
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
		return false, nil, fmt.Errorf("read %s: %w", path, err)
	}
	return allowYolo, env, nil
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
