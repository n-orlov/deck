package store

import (
	"context"
	"strings"
	"testing"
)

func TestValidateGroupName(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr string
	}{
		{"  Work  ", "Work", ""},
		{"", "", "group name is required"},
		{"   ", "", "group name is required"},
		{"Default", "", `group name "default" is reserved`},
		{" DEFAULT ", "", `group name "default" is reserved`},
		{"a\tb", "", "contains a control character"},
		{"x" + strings.Repeat("y", GroupNameMaxLength), "", "is longer than"},
		{strings.Repeat("é", GroupNameMaxLength), strings.Repeat("é", GroupNameMaxLength), ""},
	}
	for _, tc := range cases {
		got, err := validateGroupName(tc.in)
		switch {
		case tc.wantErr == "" && (err != nil || got != tc.want):
			t.Errorf("validateGroupName(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
			t.Errorf("validateGroupName(%q) error = %v; want it to contain %q", tc.in, err, tc.wantErr)
		}
	}
}

func TestGroupMutationsReportDuplicatesAndMissingGroups(t *testing.T) {
	ctx := context.Background()
	s := openValidationStore(t)
	a, err := s.CreateGroup(ctx, "Alpha")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateGroup(ctx, "Beta")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateGroup(ctx, "alpha"); err == nil || err.Error() != `group name "alpha" already exists` {
		t.Fatalf("duplicate create error = %v; want group name \"alpha\" already exists", err)
	}
	if err := s.RenameGroup(ctx, b.ID, "ALPHA"); err == nil || err.Error() != `group name "ALPHA" already exists` {
		t.Fatalf("rename onto an existing name error = %v", err)
	}
	if err := s.RenameGroup(ctx, 9999, "Gamma"); err == nil || err.Error() != "group 9999 not found" {
		t.Fatalf("rename of a missing group error = %v", err)
	}
	if err := s.RenameGroup(ctx, a.ID, "default"); err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("rename to the reserved name error = %v", err)
	}
	if err := s.DeleteGroup(ctx, 9999); err == nil || err.Error() != "group 9999 not found" {
		t.Fatalf("delete of a missing group error = %v", err)
	}
	if err := s.DeleteGroup(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	groups, err := s.ListGroups(ctx)
	if err != nil || len(groups) != 1 || groups[0] != b {
		t.Fatalf("groups after the failed calls and one delete = %+v, %v; want only %+v", groups, err, b)
	}
}
