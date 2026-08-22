package features

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/cucumber/godog"
)

// directoryEntryFingerprint captures everything requirement 29's
// destructive-path proofs need to notice about one filesystem entry: its
// kind, permission bits, modification time and (for regular files and
// symlinks) size/content. Two fingerprints of what is meant to be the same
// directory compare equal only when every entry, recursively, is
// byte-for-byte and stat-for-stat identical.
type directoryEntryFingerprint struct {
	IsDir bool
	Mode  fs.FileMode
	// ModTimeUnixNano, not time.Time, so comparison never depends on a
	// monotonic reading that a fresh os.Stat never carries anyway.
	ModTimeUnixNano int64
	Size            int64
	// Content holds a regular file's bytes, or a symlink's target string.
	// Directories leave it nil.
	Content []byte
}

// directoryFingerprint maps a directory-relative, slash-separated path (the
// root itself is never a key) to its entry fingerprint.
type directoryFingerprint map[string]directoryEntryFingerprint

// fingerprintDirectory walks root recursively and returns a fingerprint of
// every entry beneath it. A symlink is fingerprinted by its own lstat info
// plus target string rather than followed, so a destructive path that swaps
// a symlink's target is still caught rather than silently resolved through.
func fingerprintDirectory(root string) (directoryFingerprint, error) {
	fp := make(directoryFingerprint)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("stat %q: %w", rel, err)
		}
		record := directoryEntryFingerprint{
			IsDir:           entry.IsDir(),
			Mode:            info.Mode(),
			ModTimeUnixNano: info.ModTime().UnixNano(),
		}
		switch {
		case entry.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return fmt.Errorf("read symlink %q: %w", rel, err)
			}
			record.Content = []byte(target)
		case !entry.IsDir():
			content, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("read file %q: %w", rel, err)
			}
			record.Content = content
			record.Size = info.Size()
		}
		fp[rel] = record
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("fingerprint directory %q: %w", root, err)
	}
	return fp, nil
}

// compareFingerprints reports the first difference between two fingerprints
// of what is meant to be the same directory, naming the differing path so a
// failing scenario points straight at the damage. Paths are compared in
// sorted order so the reported difference is deterministic across runs.
func compareFingerprints(before, after directoryFingerprint) error {
	paths := make(map[string]struct{}, len(before)+len(after))
	for p := range before {
		paths[p] = struct{}{}
	}
	for p := range after {
		paths[p] = struct{}{}
	}
	sorted := make([]string, 0, len(paths))
	for p := range paths {
		sorted = append(sorted, p)
	}
	sort.Strings(sorted)
	for _, p := range sorted {
		beforeEntry, existedBefore := before[p]
		afterEntry, existsAfter := after[p]
		switch {
		case existedBefore && !existsAfter:
			return fmt.Errorf("path %q was removed", p)
		case !existedBefore && existsAfter:
			return fmt.Errorf("path %q was added", p)
		case beforeEntry.IsDir != afterEntry.IsDir:
			return fmt.Errorf("path %q changed kind (directory vs file)", p)
		case beforeEntry.Mode != afterEntry.Mode:
			return fmt.Errorf("path %q mode changed: %v -> %v", p, beforeEntry.Mode, afterEntry.Mode)
		case beforeEntry.ModTimeUnixNano != afterEntry.ModTimeUnixNano:
			return fmt.Errorf("path %q mtime changed", p)
		case beforeEntry.Size != afterEntry.Size:
			return fmt.Errorf("path %q size changed: %d -> %d", p, beforeEntry.Size, afterEntry.Size)
		case !bytes.Equal(beforeEntry.Content, afterEntry.Content):
			return fmt.Errorf("path %q content changed", p)
		}
	}
	return nil
}

// directoryFingerprintRecord pairs a captured fingerprint with the directory
// path it was taken of, so a later assertion re-fingerprints the same path
// rather than trusting a caller to repeat it correctly.
type directoryFingerprintRecord struct {
	path string
	fp   directoryFingerprint
}

// registerFingerprintSteps exposes requirement 29's proof primitive: any
// step that already knows a real directory path can register it under a
// label (today only the scratch-directory seeding step below does; a later
// task registers a session's own cwd the same way), and the fingerprint/
// assertion steps here operate purely on that label, so the same two steps
// serve every destructive path this phase proves (kill, delete, reap, purge,
// archive, bulk actions and every undo -- tasks 036/037).
func registerFingerprintSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a scratch directory "([^"]+)" is seeded with:$`, seedScratchDirectory)
	sc.Step(`^the directory "([^"]+)" is fingerprinted as "([^"]+)"$`, fingerprintNamedDirectory)
	sc.Step(`^the directory "([^"]+)" still matches fingerprint "([^"]+)"$`, assertNamedDirectoryMatchesFingerprint)
}

// seedScratchDirectory creates a fresh directory under the scenario's own
// DECK_HOME and populates it from a table of path/kind/content(/mode) rows,
// registering the resulting path under label for the fingerprint steps.
// kind is "file" or "dir"; mode is an optional octal string (default 0644
// for files, 0755 for directories) so a later task can seed a file with no
// write permission without a second seeding mechanism.
func seedScratchDirectory(ctx context.Context, label string, table *godog.Table) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	if len(table.Rows) < 2 {
		return fmt.Errorf("scratch directory %q seed table has no data rows", label)
	}
	col := make(map[string]int, len(table.Rows[0].Cells))
	for i, cell := range table.Rows[0].Cells {
		col[cell.Value] = i
	}
	for _, required := range []string{"path", "kind"} {
		if _, ok := col[required]; !ok {
			return fmt.Errorf("scratch directory %q seed table has no %q column", label, required)
		}
	}
	root := filepath.Join(h.Home, "fingerprint-scratch", label)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("create scratch directory %q: %w", label, err)
	}
	for _, row := range table.Rows[1:] {
		cell := func(name string) string {
			idx, ok := col[name]
			if !ok || idx >= len(row.Cells) {
				return ""
			}
			return row.Cells[idx].Value
		}
		relPath := cell("path")
		kind := cell("kind")
		full := filepath.Join(root, relPath)
		mode, err := scratchEntryMode(cell("mode"), kind)
		if err != nil {
			return fmt.Errorf("scratch directory %q entry %q: %w", label, relPath, err)
		}
		switch kind {
		case "dir":
			if err := os.MkdirAll(full, 0o755); err != nil {
				return fmt.Errorf("create scratch subdirectory %q: %w", relPath, err)
			}
			if err := os.Chmod(full, mode); err != nil {
				return fmt.Errorf("chmod scratch subdirectory %q: %w", relPath, err)
			}
		case "file":
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				return fmt.Errorf("create parent directory for scratch file %q: %w", relPath, err)
			}
			if err := os.WriteFile(full, []byte(cell("content")), 0o644); err != nil {
				return fmt.Errorf("write scratch file %q: %w", relPath, err)
			}
			if err := os.Chmod(full, mode); err != nil {
				return fmt.Errorf("chmod scratch file %q: %w", relPath, err)
			}
		default:
			return fmt.Errorf("scratch directory %q entry %q has unknown kind %q (want file or dir)", label, relPath, kind)
		}
	}
	if h.namedDirectories == nil {
		h.namedDirectories = make(map[string]string)
	}
	h.namedDirectories[label] = root
	return nil
}

func scratchEntryMode(modeStr, kind string) (os.FileMode, error) {
	if modeStr == "" {
		if kind == "dir" {
			return 0o755, nil
		}
		return 0o644, nil
	}
	parsed, err := strconv.ParseUint(modeStr, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("parse mode %q as octal: %w", modeStr, err)
	}
	return os.FileMode(parsed), nil
}

func fingerprintNamedDirectory(ctx context.Context, dirLabel, fpLabel string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	path, ok := h.namedDirectories[dirLabel]
	if !ok {
		return fmt.Errorf("directory %q has not been registered for fingerprinting", dirLabel)
	}
	fp, err := fingerprintDirectory(path)
	if err != nil {
		return err
	}
	if h.directoryFingerprints == nil {
		h.directoryFingerprints = make(map[string]directoryFingerprintRecord)
	}
	h.directoryFingerprints[fpLabel] = directoryFingerprintRecord{path: path, fp: fp}
	return nil
}

func assertNamedDirectoryMatchesFingerprint(ctx context.Context, dirLabel, fpLabel string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	path, ok := h.namedDirectories[dirLabel]
	if !ok {
		return fmt.Errorf("directory %q has not been registered for fingerprinting", dirLabel)
	}
	record, ok := h.directoryFingerprints[fpLabel]
	if !ok {
		return fmt.Errorf("no fingerprint was captured as %q", fpLabel)
	}
	if record.path != path {
		return fmt.Errorf("fingerprint %q was captured for directory %q, not %q's current path %q", fpLabel, record.path, dirLabel, path)
	}
	current, err := fingerprintDirectory(path)
	if err != nil {
		return err
	}
	if err := compareFingerprints(record.fp, current); err != nil {
		return fmt.Errorf("directory %q no longer matches fingerprint %q: %w", dirLabel, fpLabel, err)
	}
	return nil
}
