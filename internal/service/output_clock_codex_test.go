package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func (r *movementRig) tickPane(nowSec, activity int64, paneText string) {
	r.t.Helper()
	r.now = time.Unix(nowSec, 0)
	for name, content := range map[string]string{"activity": itoa(activity), "pane": paneText} {
		if err := os.WriteFile(filepath.Join(r.home, name), []byte(content), 0o600); err != nil {
			r.t.Fatal(err)
		}
	}
	if err := r.svc.ReconcileWithProbes(context.Background(), movementStale); err != nil {
		r.t.Fatal(err)
	}
}

// R304/R316/R324: a codex pane draws no separator pair, only a "›" composer and
// a status line (with a spinner) under it. Footer-only churn in that shape
// leaves last_output_at, the unseen marker and the row's movement status alone;
// a real transcript change in two consecutive buckets still counts.
func TestCodexFooterOnlyChurnLeavesOutputClockUnseenAndMovementAlone(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "agent", "testdata", "probes", "codex", "idle.txt"))
	if err != nil {
		t.Fatal(err)
	}
	idle := string(raw)
	const status = "fake-model default · /tmp/codex-spike/work"
	if !strings.Contains(idle, status) {
		t.Fatal("golden lost its status line")
	}
	r := newMovementRig(t, "codex", 2*time.Hour)
	r.tickPane(1000, 1000, idle)
	for i, footer := range []string{
		status + " · renaming... ⠋",
		"other-model high · /tmp/elsewhere · renaming... ⠙",
		status,
	} {
		now := int64(1010 + 10*i)
		churned := strings.Replace(idle, status, footer, 1)
		churned = strings.Replace(churned, "› Ask Codex to do anything", "› a half-typed draft", 1)
		r.tickPane(now, now, churned)
		got := r.row()
		if got.LastOutputAt != 0 || got.Unseen() || got.Status != "idle" {
			t.Fatalf("footer-only tick %d: last_output_at=%d unseen=%v status=%q", i, got.LastOutputAt, got.Unseen(), got.Status)
		}
	}
	r.tickPane(1040, 1040, strings.Replace(idle, "hello.txt is the only file", "two files", 1))
	if got := r.row(); got.LastOutputAt == 0 || !got.Unseen() {
		t.Fatalf("a transcript change did not count: last_output_at=%d unseen=%v", got.LastOutputAt, got.Unseen())
	}
	r.tickPane(1050, 1050, strings.Replace(idle, "hello.txt is the only file", "three files now", 1))
	r.wantStatus("running", "transcript changes in two consecutive buckets")
}
