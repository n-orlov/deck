package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Group is one row of SPEC §4's groups table -- a manually named place a
// session can belong to. There is deliberately no row for "default": SPEC
// §11's grouping model states that group as sessions.group_id IS NULL, so
// only real, user-created groups ever appear here (task 008, schemaV7).
type Group struct {
	ID   int64
	Name string
}

// GroupNameMaxLength is SPEC §11's name cap (R128). It is a sanity bound,
// not a fitting bound -- §11.2 clamps sidebar_width to [24, width-40], so
// the narrowest legal sidebar has a 20-cell content floor, and
// "▾ " + name + a two-space gap + "(999)" leaves only 11 cells for the name
// there. Elision (R129) does the real work of fitting a name into that
// floor; this cap exists only to keep a name legible in the `,` settings
// editor and the create modal's cycling field. 32 is long enough for every
// name in the operator's own examples ("tooling maintenance" is 19 runes)
// and short enough to stay readable end to end.
const GroupNameMaxLength = 32

// defaultGroupName is the reserved case-insensitive name: SPEC §11 states
// `default` is not a row (it is group_id IS NULL), and reserving the literal
// name keeps a user from creating a second, real row that would read back
// identically to the structural default in every list and picker.
const defaultGroupName = "default"

// validateGroupName rejects name when it carries a control character
// ANYWHERE -- including leading or trailing, which is why the scan runs on
// the raw input before any trimming: a group name renders verbatim in the
// sidebar header and the settings editor, and a control character there
// could corrupt either frame, so a name carrying one is refused outright
// rather than silently repaired into a different name than the operator
// typed. It then trims surrounding spaces, rejects the result when empty,
// rejects "default" in any case (SPEC §11: reserved for the structural
// group_id IS NULL row), and enforces GroupNameMaxLength. It returns the
// trimmed name callers should persist. Case-insensitive uniqueness against
// other real groups is enforced by the groups.name UNIQUE COLLATE NOCASE
// constraint (schemaV7) and translated into a plain error by the caller,
// not re-checked here.
func validateGroupName(name string) (string, error) {
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("group name %q contains a control character", name)
		}
	}
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", errors.New("group name is required")
	}
	if strings.EqualFold(trimmed, defaultGroupName) {
		return "", errors.New(`group name "default" is reserved`)
	}
	if utf8.RuneCountInString(trimmed) > GroupNameMaxLength {
		return "", fmt.Errorf("group name %q is longer than %d characters", trimmed, GroupNameMaxLength)
	}
	return trimmed, nil
}

// isGroupNameUniqueConstraintErr recognises the groups.name UNIQUE COLLATE
// NOCASE violation modernc.org/sqlite raises, the same substring-matching
// idiom RenameSession already uses for sessions.name/sessions.slug.
func isGroupNameUniqueConstraintErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "groups.name")
}

// CreateGroup inserts one new group row (SPEC §11, R128). See
// validateGroupName for the name rules; a name already held by another
// group, case-insensitively, is refused.
func (s *Store) CreateGroup(ctx context.Context, name string) (Group, error) {
	trimmed, err := validateGroupName(name)
	if err != nil {
		return Group{}, err
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO groups (name) VALUES (?)`, trimmed)
	if err != nil {
		if isGroupNameUniqueConstraintErr(err) {
			return Group{}, fmt.Errorf("group name %q already exists", trimmed)
		}
		return Group{}, fmt.Errorf("create group: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Group{}, fmt.Errorf("read created group id: %w", err)
	}
	return Group{ID: id, Name: trimmed}, nil
}

// RenameGroup updates exactly the one groups row named by id (SPEC §11,
// R128: "membership is by id, not name, so a rename is one row update and
// carries every member for free" -- sessions.group_id is untouched, so
// every session already pointing at this id resolves to the new name on its
// very next read, with no member-row rewrite at all).
func (s *Store) RenameGroup(ctx context.Context, id int64, newName string) error {
	trimmed, err := validateGroupName(newName)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE groups SET name = ? WHERE id = ?`, trimmed, id)
	if err != nil {
		if isGroupNameUniqueConstraintErr(err) {
			return fmt.Errorf("group name %q already exists", trimmed)
		}
		return fmt.Errorf("rename group: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rename group: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("group %d not found", id)
	}
	return nil
}

// DeleteGroup removes exactly the one groups row named by id. It never
// touches member sessions: groups.id carries deliberately no foreign key
// (schemaV7, task 008) precisely so a group can be deleted independently of
// what happens to its members -- SPEC §11's "move to default, or delete
// them" choice, and the destructive branch's reuse of the dd batch path
// (R131), both live above this call, not inside it. A member whose
// group_id now dangles reads back under `default` (SPEC §11) via the
// existing LEFT JOIN in sessionsFromClause, never an error.
func (s *Store) DeleteGroup(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM groups WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete group: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check delete group: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("group %d not found", id)
	}
	return nil
}

// ListGroups returns every real group row (never a synthetic "default"
// entry -- see Group's own doc comment), ordered alphabetically,
// case-insensitively, matching SPEC §11's sidebar group order (`default`
// itself sorts last there only because it is not one of these rows at all).
func (s *Store) ListGroups(ctx context.Context) ([]Group, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name FROM groups ORDER BY name COLLATE NOCASE ASC`)
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	defer rows.Close()
	var groups []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name); err != nil {
			return nil, fmt.Errorf("scan group: %w", err)
		}
		groups = append(groups, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	return groups, nil
}
