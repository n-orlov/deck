package service

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// auditLines reads the deck.jsonl lines whose event is name, as raw text and
// decoded.
func (r *sizeRig) auditLines(name string) ([]string, []map[string]any) {
	r.t.Helper()
	file, err := os.Open(r.svc.Audit.Path())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		r.t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	var raws []string
	var decoded []map[string]any
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var line map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			r.t.Fatalf("deck.jsonl line %q is not JSON: %v", scanner.Text(), err)
		}
		if line["event"] == name {
			raws = append(raws, scanner.Text())
			decoded = append(decoded, line)
		}
	}
	return raws, decoded
}

// TestOutputClockLogsWhyItWroteAndRebaselined is R413: a last_output_at write
// appends one output.recorded line saying why it wrote, a size re-baseline
// appends one output.rebaselined line, and neither carries pane text or a hash.
func TestOutputClockLogsWhyItWroteAndRebaselined(t *testing.T) {
	const (
		footer = "~/proj  main  ctx:12%  opus"
		secret = "SECRET-TRANSCRIPT-WORDS"
	)
	r := newSizeRig(t)
	r.svc.Output.Audit = r.svc.Audit

	// Baseline at 80x24, activity 2000: nothing is written, nothing is logged.
	r.tick("2000", "80|24", claudePane(80, footer), "")
	if raws, _ := r.auditLines("output.recorded"); len(raws) != 0 {
		t.Fatalf("the baseline tick logged output.recorded: %v", raws)
	}

	// A resize to 60x24: one output.rebaselined line with the old and new size.
	r.tick("2003", "60|24", claudePane(60, footer), "")
	raws, rebaselined := r.auditLines("output.rebaselined")
	if len(rebaselined) != 1 {
		t.Fatalf("a size re-baseline logged %d output.rebaselined lines, want 1: %v", len(rebaselined), raws)
	}
	line := rebaselined[0]
	if line["session_id"] != r.id || line["old_size"] != "80x24" || line["new_size"] != "60x24" {
		t.Fatalf("output.rebaselined = %v, want session %s old 80x24 new 60x24", line, r.id)
	}
	if got, _ := r.auditLines("output.recorded"); len(got) != 0 {
		t.Fatalf("a re-baseline logged output.recorded: %v", got)
	}

	// New transcript text in a later bucket at the settled size: one write.
	r.tick("2025", "60|24", strings.Replace(claudePane(60, footer), "Reading", secret, 1), "")
	if got := r.lastOutputAt(); got != 2025000 {
		t.Fatalf("last_output_at = %d, want 2025000", got)
	}
	recorded, decoded := r.auditLines("output.recorded")
	if len(decoded) != 1 {
		t.Fatalf("a last_output_at write logged %d output.recorded lines, want 1: %v", len(decoded), recorded)
	}
	line = decoded[0]
	want := map[string]any{
		"session_id": r.id, "old_last_output_at": float64(0), "new_last_output_at": float64(2025000),
		"window_activity": float64(2025), "bucket": float64(202), "pane_size": "60x24",
		"baseline_pane_size": "60x24", "discount_span": false,
	}
	for key, value := range want {
		if line[key] != value {
			t.Errorf("output.recorded %s = %v, want %v (line %s)", key, line[key], value, recorded[0])
		}
	}

	// A second write names the first as the old value.
	r.tick("2040", "60|24", strings.Replace(claudePane(60, footer), "Reading", secret+"-2", 1), "")
	_, decoded = r.auditLines("output.recorded")
	if len(decoded) != 2 || decoded[1]["old_last_output_at"] != float64(2025000) || decoded[1]["new_last_output_at"] != float64(2040000) {
		t.Fatalf("second output.recorded = %v, want old 2025000 new 2040000", decoded)
	}

	// Neither line carries pane text or a transcript hash.
	for _, name := range []string{"output.recorded", "output.rebaselined"} {
		lines, _ := r.auditLines(name)
		for _, text := range lines {
			if strings.Contains(text, secret) || strings.Contains(text, "reflow") || strings.Contains(text, "transcript") {
				t.Errorf("%s carries pane text: %s", name, text)
			}
			for _, field := range strings.FieldsFunc(text, func(c rune) bool { return (c < '0' || c > '9') && (c < 'a' || c > 'f') }) {
				if len(field) >= 32 {
					t.Errorf("%s carries a hash-like value %q: %s", name, field, text)
				}
			}
		}
	}
}
