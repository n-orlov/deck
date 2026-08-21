package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// FileConfig is what loadConfigFile returns: one field per Schema entry,
// with every key that was absent from the file already filled in from its
// schema Default. loadConfigFile itself never invents a key or a parsing
// rule -- for each key found in the file's top-level or [ui] table, it
// looks the key up in Schema and dispatches on the found Field's Kind, so
// task 011's parser has exactly one generic rule per FieldKind rather than
// a hand-written case per key. [env] is handled separately: Schema
// declares it as a single whole-table field (KindListOfStrings, arbitrary
// member names), so its members go straight into Env.
type FileConfig struct {
	AllowYolo          bool
	StaleAfter         time.Duration
	CaptureMinInterval time.Duration
	ASCII              bool
	Mouse              bool
	RecentCwdLimit     int
	Theme              string
	Env                map[string]string
}

// loadConfigFile reads config.toml's implemented top-level controls, the
// [ui] table and the [env] table. It intentionally does not attempt a
// general TOML parser -- deck's config.toml also carries [notify] and
// [[notify.rule]] tables (SPEC §10) that are out of scope here, so
// unrecognised sections are skipped rather than misparsed. A missing file
// yields the Schema's documented defaults and no error; a file that exists
// but cannot be understood yields a stated error naming the file and line,
// never a silent default. A key present in a known section (top level,
// [ui]) but absent from Schema is ignored, the same way a future addition
// to Schema needs no matching edit here to be picked up.
func loadConfigFile(path string) (FileConfig, error) {
	cfg := defaultFileConfig()

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return FileConfig{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	section := ""
	scanner := bufio.NewScanner(file)
	line := 0
	for scanner.Scan() {
		line++
		raw := scanner.Text()
		text := strings.TrimSpace(stripComment(raw))
		if text == "" {
			continue
		}
		if strings.HasPrefix(text, "[") {
			name, err := parseSectionHeader(text)
			if err != nil {
				return FileConfig{}, fmt.Errorf("%s:%d: %w", path, line, err)
			}
			section = name
			continue
		}
		key, value, err := parseKeyValue(text)
		if err != nil {
			return FileConfig{}, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		switch section {
		case "", "ui":
			fullKey := key
			if section != "" {
				fullKey = section + "." + key
			}
			field, ok := FieldByFullKey(fullKey)
			if !ok {
				// A key with no matching schema entry is ignored so future
				// phases can add keys without breaking this parser, and so
				// a typo does not silently masquerade as a known key.
				continue
			}
			if err := setField(&cfg, field, value, path, line); err != nil {
				return FileConfig{}, err
			}
		case "env":
			unquoted, err := unquoteString(value)
			if err != nil {
				return FileConfig{}, fmt.Errorf("%s:%d: [env] value for %q must be a quoted string: %w", path, line, key, err)
			}
			if cfg.Env == nil {
				cfg.Env = make(map[string]string)
			}
			cfg.Env[key] = unquoted
		default:
			// A recognised-but-out-of-scope section (e.g. [notify]): its
			// body is intentionally not interpreted.
		}
	}
	if err := scanner.Err(); err != nil {
		return FileConfig{}, fmt.Errorf("read %s: %w", path, err)
	}
	return cfg, nil
}

// defaultFileConfig seeds every field from Schema's declared Default,
// rather than repeating each default as a separate literal, so the
// defaults a missing (or partially populated) config.toml yields can never
// drift from what Schema documents.
func defaultFileConfig() FileConfig {
	var cfg FileConfig
	for _, field := range Schema {
		switch field.FullKey() {
		case "allow_yolo":
			cfg.AllowYolo, _ = field.Default.(bool)
		case "stale_after":
			seconds, _ := field.Default.(int)
			cfg.StaleAfter = time.Duration(seconds) * time.Second
		case "capture_min_interval":
			seconds, _ := field.Default.(int)
			cfg.CaptureMinInterval = time.Duration(seconds) * time.Second
		case "ui.ascii":
			cfg.ASCII, _ = field.Default.(bool)
		case "ui.mouse":
			cfg.Mouse, _ = field.Default.(bool)
		case "ui.recent_cwd_limit":
			cfg.RecentCwdLimit, _ = field.Default.(int)
		case "ui.theme":
			cfg.Theme, _ = field.Default.(string)
		}
	}
	return cfg
}

// setField parses raw against field's declared Kind and, once valid,
// writes it into cfg's matching member. The Kind switch is the one generic
// parsing rule per kind that replaces the former per-key switch; the
// FullKey switch beneath it exists only because Go structs cannot be
// addressed by a string field name without reflection, so it carries no
// parsing logic of its own -- everything that can fail already failed
// above it.
func setField(cfg *FileConfig, field Field, raw, path string, line int) error {
	switch field.Kind {
	case KindToggle:
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("%s:%d: %s must be true or false, got %q", path, line, field.FullKey(), raw)
		}
		switch field.FullKey() {
		case "allow_yolo":
			cfg.AllowYolo = value
		case "ui.ascii":
			cfg.ASCII = value
		case "ui.mouse":
			cfg.Mouse = value
		}
	case KindInteger:
		value, err := parseIntegerValue(field, raw)
		if err != nil {
			return fmt.Errorf("%s:%d: %w", path, line, err)
		}
		switch field.FullKey() {
		case "stale_after":
			cfg.StaleAfter = time.Duration(value) * time.Second
		case "capture_min_interval":
			cfg.CaptureMinInterval = time.Duration(value) * time.Second
		case "ui.recent_cwd_limit":
			cfg.RecentCwdLimit = value
		}
	case KindEnum, KindString, KindPath:
		unquoted, err := unquoteString(raw)
		if err != nil {
			return fmt.Errorf("%s:%d: %s must be a quoted string: %w", path, line, field.FullKey(), err)
		}
		switch field.FullKey() {
		case "ui.theme":
			cfg.Theme = unquoted
		}
	default:
		return fmt.Errorf("%s:%d: %s: unsupported field kind %q for a flat key", path, line, field.FullKey(), field.Kind)
	}
	return nil
}

// parseIntegerValue parses raw against field's IntBounds. A field whose
// Unit is "seconds" additionally accepts a quoted Go duration string
// (e.g. "1m30s") alongside a bare integer, preserving stale_after's
// pre-schema behaviour and extending it to any other seconds-denominated
// integer field (currently capture_min_interval) rather than special-casing
// one key.
func parseIntegerValue(field Field, raw string) (int, error) {
	var value int
	if field.Unit == "seconds" && strings.HasPrefix(raw, "\"") {
		text, err := unquoteString(raw)
		if err != nil {
			return 0, fmt.Errorf("%s must be seconds or a duration, got %q", field.FullKey(), raw)
		}
		duration, err := time.ParseDuration(text)
		if err != nil {
			return 0, fmt.Errorf("%s must be seconds or a duration, got %q", field.FullKey(), raw)
		}
		value = int(duration.Seconds())
	} else {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			if field.Unit == "seconds" {
				return 0, fmt.Errorf("%s must be seconds or a duration, got %q", field.FullKey(), raw)
			}
			return 0, fmt.Errorf("%s must be an integer, got %q", field.FullKey(), raw)
		}
		value = parsed
	}
	if value < field.IntBounds.Min {
		return 0, fmt.Errorf("%s must be at least %d, got %d", field.FullKey(), field.IntBounds.Min, value)
	}
	if field.IntBounds.Max != nil && value > *field.IntBounds.Max {
		return 0, fmt.Errorf("%s must be at most %d, got %d", field.FullKey(), *field.IntBounds.Max, value)
	}
	return value, nil
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
