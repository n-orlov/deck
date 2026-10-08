package config

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"slices"
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
	AllowYolo            bool
	YoloDefault          bool
	StaleAfter           time.Duration
	CaptureMinInterval   time.Duration
	InteractiveInterval  time.Duration
	InteractiveTransport string
	TmuxMouse            bool
	ASCII                bool
	Mouse                bool
	DefaultGroupFirst    bool
	PreviewFit           bool
	AttachOnNew          bool
	AttachOnClick        bool
	SelectOnDrag         bool
	AttachOnResume       bool
	PreviewPaint         string
	SortOrder            string
	RecentCwdLimit       int
	EventRetentionDays   int
	Theme                string
	PreLaunch            string
	PostDestroy          string
	Agent                string
	EventHook            string
	EventHookDefault     bool
	EventHookEvents      []string
	EventHookTimeout     time.Duration
	Env                  map[string]string
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

	file, err := os.Open(path) //nolint:gosec // G304: the config path is the resolved config.toml location (or the profile file the operator named)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return FileConfig{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = file.Close() }() // read-only handle: Close cannot lose data

	if err := parseConfigStream(file, path, &cfg); err != nil {
		return FileConfig{}, err
	}
	return cfg, nil
}

// parseConfigStream reads the TOML subset line by line from r, applying
// every key/value pair to cfg. path and the 1-based line number prefix each
// parse error; a read failure is reported as "read <path>".
func parseConfigStream(r io.Reader, path string, cfg *FileConfig) error {
	section := ""
	scanner := bufio.NewScanner(r)
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
				return fmt.Errorf("%s:%d: %w", path, line, err)
			}
			section = name
			continue
		}
		key, value, err := parseKeyValue(text)
		if err != nil {
			return fmt.Errorf("%s:%d: %w", path, line, err)
		}
		if err := applyConfigKey(cfg, section, key, value, path, line); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	return nil
}

// applyConfigKey applies one parsed key/value pair to cfg according to the
// section it appeared in: top level and [ui] keys go through Schema, [env]
// keys land in cfg.Env, and any other section's body is not interpreted.
func applyConfigKey(cfg *FileConfig, section, key, value, path string, line int) error {
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
			return nil
		}
		return setField(cfg, field, value, path, line)
	case "env":
		unquoted, err := unquoteString(value)
		if err != nil {
			return fmt.Errorf("%s:%d: [env] value for %q must be a quoted string: %w", path, line, key, err)
		}
		if cfg.Env == nil {
			cfg.Env = make(map[string]string)
		}
		cfg.Env[key] = unquoted
	default:
		// A recognised-but-out-of-scope section (e.g. [notify]): its
		// body is intentionally not interpreted.
	}
	return nil
}

// defaultFileConfig seeds every field from Schema's declared Default,
// rather than repeating each default as a separate literal, so the
// defaults a missing (or partially populated) config.toml yields can never
// drift from what Schema documents.
func defaultFileConfig() FileConfig {
	var cfg FileConfig
	for _, field := range Schema {
		seedDefault(&cfg, field)
	}
	return cfg
}

// seedDefault writes field's declared Default into cfg through the setter
// table that owns the field's FullKey. A Default of an unexpected type
// seeds the zero value; a key no table owns (an [env] field) is ignored.
func seedDefault(cfg *FileConfig, field Field) {
	key := field.FullKey()
	if set := toggleSetters[key]; set != nil {
		value, _ := field.Default.(bool)
		set(cfg, value)
	}
	if set := integerSetters[key]; set != nil {
		value, _ := field.Default.(int)
		set(cfg, value)
	}
	if set := stringSetters[key]; set != nil {
		value, _ := field.Default.(string)
		set(cfg, value)
	}
	if set := listSetters[key]; set != nil {
		value, _ := field.Default.([]string)
		set(cfg, slices.Clone(value)) // never alias the Schema's own default slice
	}
}

// toggleSetters, integerSetters and stringSetters map a field's FullKey to
// the FileConfig member it writes. Go structs cannot be addressed by a
// string field name without reflection, so these tables carry no parsing
// logic of their own -- everything that can fail already failed before a
// setter is looked up. A FullKey absent from a table is ignored.
var toggleSetters = map[string]func(*FileConfig, bool){
	"allow_yolo":             func(c *FileConfig, v bool) { c.AllowYolo = v },
	"yolo_default":           func(c *FileConfig, v bool) { c.YoloDefault = v },
	"tmux_mouse":             func(c *FileConfig, v bool) { c.TmuxMouse = v },
	"ui.ascii":               func(c *FileConfig, v bool) { c.ASCII = v },
	"ui.mouse":               func(c *FileConfig, v bool) { c.Mouse = v },
	"ui.default_group_first": func(c *FileConfig, v bool) { c.DefaultGroupFirst = v },
	"ui.preview_fit":         func(c *FileConfig, v bool) { c.PreviewFit = v },
	"ui.attach_on_new":       func(c *FileConfig, v bool) { c.AttachOnNew = v },
	"ui.attach_on_click":     func(c *FileConfig, v bool) { c.AttachOnClick = v },
	"ui.select_on_drag":      func(c *FileConfig, v bool) { c.SelectOnDrag = v },
	"ui.attach_on_resume":    func(c *FileConfig, v bool) { c.AttachOnResume = v },
	"event_hook_default":     func(c *FileConfig, v bool) { c.EventHookDefault = v },
}

var integerSetters = map[string]func(*FileConfig, int){
	"stale_after":          func(c *FileConfig, v int) { c.StaleAfter = time.Duration(v) * time.Second },
	"capture_min_interval": func(c *FileConfig, v int) { c.CaptureMinInterval = time.Duration(v) * time.Second },
	"interactive_ms":       func(c *FileConfig, v int) { c.InteractiveInterval = time.Duration(v) * time.Millisecond },
	"ui.recent_cwd_limit":  func(c *FileConfig, v int) { c.RecentCwdLimit = v },
	"event_retention_days": func(c *FileConfig, v int) { c.EventRetentionDays = v },
	"event_hook_timeout":   func(c *FileConfig, v int) { c.EventHookTimeout = time.Duration(v) * time.Second },
}

var stringSetters = map[string]func(*FileConfig, string){
	"ui.theme":              func(c *FileConfig, v string) { c.Theme = v },
	"ui.sort_order":         func(c *FileConfig, v string) { c.SortOrder = v },
	"ui.preview_paint":      func(c *FileConfig, v string) { c.PreviewPaint = v },
	"interactive_transport": func(c *FileConfig, v string) { c.InteractiveTransport = v },
	"pre_launch":            func(c *FileConfig, v string) { c.PreLaunch = v },
	"post_destroy":          func(c *FileConfig, v string) { c.PostDestroy = v },
	"agent":                 func(c *FileConfig, v string) { c.Agent = v },
	"event_hook":            func(c *FileConfig, v string) { c.EventHook = v },
}

// listSetters is the KindListOfStrings counterpart of stringSetters for the
// flat list keys ([env] is a table and never goes through here).
var listSetters = map[string]func(*FileConfig, []string){
	"event_hook_events": func(c *FileConfig, v []string) { c.EventHookEvents = v },
}

// setField parses raw against field's declared Kind and, once valid,
// writes it into cfg's matching member through the per-kind setter table.
func setField(cfg *FileConfig, field Field, raw, path string, line int) error {
	switch field.Kind {
	case KindToggle:
		return setToggleField(cfg, field, raw, path, line)
	case KindInteger:
		return setIntegerField(cfg, field, raw, path, line)
	case KindEnum, KindString, KindPath:
		return setStringField(cfg, field, raw, path, line)
	case KindListOfStrings:
		return setListField(cfg, field, raw, path, line)
	default:
		return fmt.Errorf("%s:%d: %s: unsupported field kind %q for a flat key", path, line, field.FullKey(), field.Kind)
	}
}

// setToggleField parses raw as a boolean and writes it through the toggle
// setter table.
func setToggleField(cfg *FileConfig, field Field, raw, path string, line int) error {
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return fmt.Errorf("%s:%d: %s must be true or false, got %q", path, line, field.FullKey(), raw)
	}
	if set := toggleSetters[field.FullKey()]; set != nil {
		set(cfg, value)
	}
	return nil
}

// setIntegerField parses raw against the field's bounds and writes it
// through the integer setter table.
func setIntegerField(cfg *FileConfig, field Field, raw, path string, line int) error {
	value, err := parseIntegerValue(field, raw)
	if err != nil {
		return fmt.Errorf("%s:%d: %w", path, line, err)
	}
	if set := integerSetters[field.FullKey()]; set != nil {
		set(cfg, value)
	}
	return nil
}

// setListField parses raw as a one-line TOML array of quoted strings,
// checks every element against the field's ElementValues and writes the
// list through the list setter table.
func setListField(cfg *FileConfig, field Field, raw, path string, line int) error {
	items, err := parseStringArray(raw)
	if err != nil {
		return fmt.Errorf("%s:%d: %s must be an array of quoted strings: %w", path, line, field.FullKey(), err)
	}
	if err := CheckListElements(field, items); err != nil {
		return fmt.Errorf("%s:%d: %w", path, line, err)
	}
	if set := listSetters[field.FullKey()]; set != nil {
		set(cfg, items)
	}
	return nil
}

// CheckListElements reports the first element of items outside field's
// ElementValues, as an error naming the key. A field with no ElementValues
// accepts any element. The settings dialog uses it to refuse the same
// words the file loader refuses.
func CheckListElements(field Field, items []string) error {
	if len(field.ElementValues) == 0 {
		return nil
	}
	for _, item := range items {
		if !slices.Contains(field.ElementValues, item) {
			return fmt.Errorf("%s: unknown event kind %q, must be one of %s", field.FullKey(), item, strings.Join(field.ElementValues, ", "))
		}
	}
	return nil
}

// CheckEventKinds refuses a session's own event_hook_events list (SPEC §10.2)
// when it names a kind outside the offered set of §10.1, with the same
// message the settings dialog and the file loader give.
func CheckEventKinds(items []string) error {
	return CheckListElements(Field{Key: "event_hook_events", Kind: KindListOfStrings, ElementValues: EventHookKinds}, items)
}

// parseStringArray reads a one-line TOML array of quoted strings such as
// ["waiting", "error"]; [] is the empty list and a trailing comma is allowed.
func parseStringArray(raw string) ([]string, error) {
	if len(raw) < 2 || raw[0] != '[' || raw[len(raw)-1] != ']' {
		return nil, fmt.Errorf("not an array: %q", raw)
	}
	body := strings.TrimSpace(raw[1 : len(raw)-1])
	items := []string{}
	if body == "" {
		return items, nil
	}
	for _, part := range strings.Split(strings.TrimSuffix(body, ","), ",") {
		item, err := unquoteString(strings.TrimSpace(part))
		if err != nil {
			return nil, err
		}
		if strings.Contains(item, "\"") {
			return nil, fmt.Errorf("elements must be separated by commas, got %q", part)
		}
		items = append(items, item)
	}
	return items, nil
}

// setStringField unquotes and validates raw and writes it through the
// string setter table.
func setStringField(cfg *FileConfig, field Field, raw, path string, line int) error {
	unquoted, err := parseStringFieldValue(field, raw, path, line)
	if err != nil {
		return err
	}
	if set := stringSetters[field.FullKey()]; set != nil {
		set(cfg, unquoted)
	}
	return nil
}

// parseStringFieldValue unquotes raw and, for a KindEnum field with a
// statically declared, non-dynamic choice set (unlike ui.theme's
// DynamicEnum, whose choices depend on runtime theme discovery and so are
// validated by theme.Resolve instead), rejects a value outside that set at
// parse time, exactly like an out-of-bounds integer is -- never silently
// coerced to the default.
func parseStringFieldValue(field Field, raw, path string, line int) (string, error) {
	unquoted, err := unquoteString(raw)
	if err != nil {
		return "", fmt.Errorf("%s:%d: %s must be a quoted string: %w", path, line, field.FullKey(), err)
	}
	if field.Kind == KindEnum && !field.DynamicEnum && len(field.EnumValues) > 0 && !slices.Contains(field.EnumValues, unquoted) {
		return "", fmt.Errorf("%s:%d: %s must be one of %v, got %q", path, line, field.FullKey(), field.EnumValues, unquoted)
	}
	return unquoted, nil
}

// parseIntegerValue parses raw against field's IntBounds. A field whose
// Unit is "seconds" additionally accepts a quoted Go duration string
// (e.g. "1m30s") alongside a bare integer, preserving stale_after's
// pre-schema behaviour and extending it to any other seconds-denominated
// integer field (currently capture_min_interval) rather than special-casing
// one key.
func parseIntegerValue(field Field, raw string) (int, error) {
	value, err := parseIntegerLiteral(field, raw)
	if err != nil {
		return 0, err
	}
	if value < field.IntBounds.Min {
		return 0, fmt.Errorf("%s must be at least %d, got %d", field.FullKey(), field.IntBounds.Min, value)
	}
	if field.IntBounds.Max != nil && value > *field.IntBounds.Max {
		return 0, fmt.Errorf("%s must be at most %d, got %d", field.FullKey(), *field.IntBounds.Max, value)
	}
	return value, nil
}

// parseIntegerLiteral reads raw as a bare integer or, for a seconds field,
// as a quoted Go duration, without applying the field's bounds.
func parseIntegerLiteral(field Field, raw string) (int, error) {
	if field.Unit == "seconds" && strings.HasPrefix(raw, "\"") {
		text, err := unquoteString(raw)
		if err != nil {
			return 0, fmt.Errorf("%s must be seconds or a duration, got %q", field.FullKey(), raw)
		}
		duration, err := time.ParseDuration(text)
		if err != nil {
			return 0, fmt.Errorf("%s must be seconds or a duration, got %q", field.FullKey(), raw)
		}
		return int(duration.Seconds()), nil
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		if field.Unit == "seconds" {
			return 0, fmt.Errorf("%s must be seconds or a duration, got %q", field.FullKey(), raw)
		}
		return 0, fmt.Errorf("%s must be an integer, got %q", field.FullKey(), raw)
	}
	return parsed, nil
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
