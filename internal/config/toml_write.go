package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// WriteConfigFile serialises cfg's schema-declared fields back into path,
// writing atomically (temp file in the same directory, then rename) so a
// crash or a failed write never leaves config.toml partially written or
// unparseable -- either the rename happens and the new content is fully
// there, or it does not and the previous file (if any) is untouched.
//
// Everything this package's reader does not understand -- an unrecognised
// top-level or [ui] key, and any section other than "", "ui" and "env"
// (notably [notify] and repeated [[notify.rule]] tables, SPEC §10) -- is
// preserved byte-for-byte from the existing file, in its original
// position, because the writer only ever replaces or appends lines it can
// attribute to a Schema field.
func WriteConfigFile(path string, cfg FileConfig) error {
	blocks, err := readBlocks(path)
	if err != nil {
		return err
	}
	blocks = applyFlatFields(blocks, cfg)
	blocks = applyEnvFields(blocks, cfg)

	var buf strings.Builder
	for _, b := range blocks {
		if b.header != "" {
			buf.WriteString(b.header)
			if !strings.HasSuffix(b.header, "\n") {
				buf.WriteString("\n")
			}
		}
		for _, line := range b.lines {
			buf.WriteString(line)
			if !strings.HasSuffix(line, "\n") {
				buf.WriteString("\n")
			}
		}
	}

	return atomicWrite(path, []byte(buf.String()))
}

// tomlBlock is one section's worth of raw lines: everything from a section
// header (exclusive) up to the next section header. The very first block in
// a file has an empty header, representing the implicit top-level table.
type tomlBlock struct {
	header string // "" for the implicit top-level block, else the raw "[name]" line
	name   string // parsed section name ("" for top-level), "" also if header could not be parsed
	lines  []string
}

// readBlocks parses path into an ordered list of tomlBlock, or a single
// empty top-level block if the file does not exist yet. It never returns an
// error for a missing file (loadConfigFile's own contract), but does for a
// file that exists but cannot be read.
func readBlocks(path string) ([]tomlBlock, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []tomlBlock{{}}, nil
		}
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	rawLines := strings.Split(string(data), "\n")
	// strings.Split on a trailing-newline file yields a final "" element;
	// drop it so we do not synthesise an extra blank line on every write.
	if len(rawLines) > 0 && rawLines[len(rawLines)-1] == "" {
		rawLines = rawLines[:len(rawLines)-1]
	}

	blocks := []tomlBlock{{}}
	for _, raw := range rawLines {
		text := strings.TrimSpace(stripComment(raw))
		if strings.HasPrefix(text, "[") && strings.HasSuffix(text, "]") {
			name := strings.Trim(text, "[]")
			name = strings.TrimSpace(name)
			blocks = append(blocks, tomlBlock{header: raw, name: name})
			continue
		}
		last := &blocks[len(blocks)-1]
		last.lines = append(last.lines, raw)
	}
	return blocks, nil
}

// applyFlatFields rewrites, in place, every existing line in the ""
// (top-level) and "ui" blocks that names a Schema field found in cfg, and
// appends any such field that was not already present. Lines this function
// does not recognise as a Schema field -- an unknown key, a comment, a
// blank line -- are left exactly as they were.
func applyFlatFields(blocks []tomlBlock, cfg FileConfig) []tomlBlock {
	fieldsBySection := map[string][]Field{}
	for _, field := range Schema {
		if field.Key == "" {
			continue // the [env] whole-table field; handled separately
		}
		fieldsBySection[field.Section] = append(fieldsBySection[field.Section], field)
	}

	for _, section := range []string{"", "ui"} {
		fields := fieldsBySection[section]
		if len(fields) == 0 {
			continue
		}
		seen := map[string]bool{}
		blockIdx := blockIndexForSection(blocks, section)
		if blockIdx == -1 {
			// No existing [ui] table (top-level "" always exists as the
			// first block): create one to hold the fields.
			blocks = append(blocks, tomlBlock{header: "[" + section + "]"})
			blockIdx = len(blocks) - 1
		}
		block := &blocks[blockIdx]
		for i, raw := range block.lines {
			text := strings.TrimSpace(stripComment(raw))
			if text == "" || strings.HasPrefix(text, "[") {
				continue
			}
			key, _, err := parseKeyValue(text)
			if err != nil {
				continue
			}
			fullKey := key
			if section != "" {
				fullKey = section + "." + key
			}
			field, ok := FieldByFullKey(fullKey)
			if !ok {
				continue // unknown key: leave the line untouched
			}
			block.lines[i] = key + " = " + serializeFieldValue(field, cfg)
			seen[fullKey] = true
		}
		for _, field := range fields {
			if seen[field.FullKey()] {
				continue
			}
			block.lines = append(block.lines, field.Key+" = "+serializeFieldValue(field, cfg))
		}
	}
	return blocks
}

// applyEnvFields rewrites the [env] table to exactly match cfg.Env: an
// existing entry whose key is still in cfg.Env gets its value updated in
// place, an entry whose key is no longer in cfg.Env is dropped (the [env]
// table is declared as one whole-table Schema field, so its serialised
// content is authoritative for that field, unlike an unrecognised key
// living in "" or "ui"), and a key newly present in cfg.Env is appended in
// sorted order for a deterministic write.
func applyEnvFields(blocks []tomlBlock, cfg FileConfig) []tomlBlock {
	blockIdx := blockIndexForSection(blocks, "env")
	if blockIdx == -1 {
		if len(cfg.Env) == 0 {
			return blocks
		}
		blocks = append(blocks, tomlBlock{header: "[env]"})
		blockIdx = len(blocks) - 1
	}
	block := &blocks[blockIdx]

	seen := map[string]bool{}
	var kept []string
	for _, raw := range block.lines {
		text := strings.TrimSpace(stripComment(raw))
		if text == "" {
			kept = append(kept, raw)
			continue
		}
		key, _, err := parseKeyValue(text)
		if err != nil {
			kept = append(kept, raw)
			continue
		}
		value, ok := cfg.Env[key]
		if !ok {
			continue // key removed from cfg.Env: drop the line
		}
		kept = append(kept, key+" = "+strconv.Quote(value))
		seen[key] = true
	}
	var newKeys []string
	for key := range cfg.Env {
		if !seen[key] {
			newKeys = append(newKeys, key)
		}
	}
	sort.Strings(newKeys)
	for _, key := range newKeys {
		kept = append(kept, key+" = "+strconv.Quote(cfg.Env[key]))
	}
	block.lines = kept
	return blocks
}

// blockIndexForSection returns the index of the LAST block whose parsed
// name matches section, or -1 if none exists. The last match is used so a
// (malformed, repeated) section is extended at its final occurrence rather
// than its first.
func blockIndexForSection(blocks []tomlBlock, section string) int {
	if section == "" {
		return 0 // the implicit top-level block is always blocks[0]
	}
	found := -1
	for i := 1; i < len(blocks); i++ {
		if blocks[i].name == section {
			found = i
		}
	}
	return found
}

// serializeFieldValue renders field's current value out of cfg as a bare
// TOML right-hand side (no trailing newline).
func serializeFieldValue(field Field, cfg FileConfig) string {
	switch field.FullKey() {
	case "allow_yolo":
		return strconv.FormatBool(cfg.AllowYolo)
	case "stale_after":
		return strconv.Itoa(int(cfg.StaleAfter.Seconds()))
	case "capture_min_interval":
		return strconv.Itoa(int(cfg.CaptureMinInterval.Seconds()))
	case "ui.ascii":
		return strconv.FormatBool(cfg.ASCII)
	case "ui.mouse":
		return strconv.FormatBool(cfg.Mouse)
	case "ui.recent_cwd_limit":
		return strconv.Itoa(cfg.RecentCwdLimit)
	case "ui.theme":
		return strconv.Quote(cfg.Theme)
	default:
		// Unreachable for a well-formed Schema: every flat (non-[env])
		// field is one of the cases above. Fall back to the field's
		// declared default rather than panic.
		return fmt.Sprintf("%v", field.Default)
	}
}

// atomicWrite writes data to a temp file created alongside path, then
// renames it over path. If any step before the rename fails, path is
// never touched; a rename is used specifically because POSIX guarantees it
// is atomic within one filesystem, so a reader never observes a partially
// written file.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".config.toml.*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	// If anything below fails, remove the half-written temp file rather
	// than leaving it behind; the rename is the only step that can affect
	// the real path.
	succeeded := false
	defer func() {
		if !succeeded {
			os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", tmpPath, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmpPath, err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return fmt.Errorf("chmod %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename %s to %s: %w", tmpPath, path, err)
	}
	succeeded = true
	return nil
}
