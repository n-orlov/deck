package config

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestOutputBucketDefaultFileAndEnvOverride pins R306's key: ui.output_bucket
// is 10 s when absent, takes a bare integer (seconds) or a quoted duration
// from the file, and is outranked by DECK_OUTPUT_BUCKET.
func TestOutputBucketDefaultFileAndEnvOverride(t *testing.T) {
	field, ok := FieldByFullKey("ui.output_bucket")
	if !ok || field.Kind != KindInteger || field.Default != 10 || field.Unit != "seconds" || field.IntBounds.Min != 1 {
		t.Fatalf("schema entry for ui.output_bucket = %+v, %v; want a seconds integer, default 10, minimum 1", field, ok)
	}
	if field.Scope != ScopeGlobal {
		t.Fatalf("ui.output_bucket scope = %q, want global (applies live)", field.Scope)
	}
	def, err := LoadFrom(environment(map[string]string{"DECK_HOME": t.TempDir()}), fakeHome)
	if err != nil || def.OutputBucket != 10*time.Second || def.File.OutputBucket != 10*time.Second {
		t.Fatalf("default OutputBucket = %v / %v, %v; want 10s", def.OutputBucket, def.File.OutputBucket, err)
	}
	if _, overridden := def.EnvOverrides["ui.output_bucket"]; overridden {
		t.Fatal("ui.output_bucket reported as env-overridden with no DECK_OUTPUT_BUCKET set")
	}
	dir := writeConfigFile(t, "[ui]\noutput_bucket = 4\n")
	fromFile, err := LoadFrom(environment(map[string]string{"DECK_HOME": dir}), fakeHome)
	if err != nil || fromFile.OutputBucket != 4*time.Second {
		t.Fatalf("output_bucket = 4 in the file gave %v, %v", fromFile.OutputBucket, err)
	}
	dirDur := writeConfigFile(t, "[ui]\noutput_bucket = \"1m30s\"\n")
	fromDur, err := LoadFrom(environment(map[string]string{"DECK_HOME": dirDur}), fakeHome)
	if err != nil || fromDur.OutputBucket != 90*time.Second {
		t.Fatalf("output_bucket = \"1m30s\" gave %v, %v; want 90s", fromDur.OutputBucket, err)
	}
	over, err := LoadFrom(environment(map[string]string{"DECK_HOME": dir, "DECK_OUTPUT_BUCKET": "7"}), fakeHome)
	if err != nil || over.OutputBucket != 7*time.Second || over.File.OutputBucket != 4*time.Second {
		t.Fatalf("DECK_OUTPUT_BUCKET=7 over the file gave %v (file %v), %v; want 7s over 4s", over.OutputBucket, over.File.OutputBucket, err)
	}
	if got := over.EnvOverrides["ui.output_bucket"]; got != "DECK_OUTPUT_BUCKET" {
		t.Fatalf("EnvOverrides[ui.output_bucket] = %q, want DECK_OUTPUT_BUCKET", got)
	}
	for _, bad := range []string{"0", "-3", "soon"} {
		if _, err := LoadFrom(environment(map[string]string{"DECK_HOME": dir, "DECK_OUTPUT_BUCKET": bad}), fakeHome); err == nil || !strings.Contains(err.Error(), "DECK_OUTPUT_BUCKET") {
			t.Errorf("DECK_OUTPUT_BUCKET=%s: err = %v, want one naming the variable", bad, err)
		}
	}
}

// TestOutputBucketBelowOneSecondIsALoadErrorNamingTheKey pins the minimum:
// 0, a negative value and a sub-second duration are all refused.
func TestOutputBucketBelowOneSecondIsALoadErrorNamingTheKey(t *testing.T) {
	for _, value := range []string{"0", "-5", "\"500ms\""} {
		_, err := loadEventHookBody(t, "[ui]\noutput_bucket = "+value+"\n")
		if err == nil || !strings.Contains(err.Error(), "ui.output_bucket must be at least 1") {
			t.Errorf("value %s: err = %v, want \"ui.output_bucket must be at least 1\"", value, err)
		}
	}
}

// TestOutputBucketRoundTripsThroughWriteConfigFile writes a non-default
// bucket and reads it back, keeping the keys around it.
func TestOutputBucketRoundTripsThroughWriteConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("some_future_key = \"x\"\n\n[ui]\noutput_bucket = 3\nsome_future_ui_key = 7\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfigFile(path)
	if err != nil || cfg.OutputBucket != 3*time.Second {
		t.Fatalf("loaded OutputBucket = %v, %v; want 3s", cfg.OutputBucket, err)
	}
	cfg.OutputBucket = 25 * time.Second
	if err := WriteConfigFile(path, cfg); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"output_bucket = 25", "some_future_key = \"x\"", "some_future_ui_key = 7"} {
		if !strings.Contains(string(written), want) {
			t.Errorf("written config.toml lacks %q:\n%s", want, written)
		}
	}
	reread, err := loadConfigFile(path)
	if err != nil || reread.OutputBucket != 25*time.Second {
		t.Fatalf("re-read OutputBucket = %v, %v; want 25s", reread.OutputBucket, err)
	}
}

// TestReloadingAnOutputBucketChangeWritesNothing proves a config reload that
// picks up a new output_bucket leaves config.toml byte- and mtime-identical.
func TestReloadingAnOutputBucketChangeWritesNothing(t *testing.T) {
	f := newReloadFixture(t)
	f.write(f.config, "[ui]\noutput_bucket = 5\n")
	f.poll()
	f.write(f.config, "[ui]\noutput_bucket = 20\n")
	before, err := os.ReadFile(f.config)
	if err != nil {
		t.Fatal(err)
	}
	beforeInfo, _ := os.Stat(f.config)
	added, settings, changed := f.poll()
	if added != 1 || !changed || settings.OutputBucket != 20*time.Second {
		t.Fatalf("reload: loads=%d changed=%v OutputBucket=%v; want one reload to 20s", added, changed, settings.OutputBucket)
	}
	after, _ := os.ReadFile(f.config)
	afterInfo, _ := os.Stat(f.config)
	if string(before) != string(after) || !afterInfo.ModTime().Equal(beforeInfo.ModTime()) {
		t.Fatal("a reload that applied output_bucket rewrote config.toml")
	}
}

// R306: a valid quoted duration of at least one second keeps its exact value
// (no truncation to whole seconds), whatever the unit it is written in, and
// both Load and the config.toml writer carry it unchanged.
func TestOutputBucketQuotedDurationKeepsItsExactValue(t *testing.T) {
	cases := map[string]time.Duration{
		`"1500ms"`: 1500 * time.Millisecond, // the finding's case
		`"1.5s"`:   1500 * time.Millisecond,
		`"2.25s"`:  2250 * time.Millisecond, // a case the finding did not list
		`"1000ms"`: time.Second,
		`"1.001s"`: 1001 * time.Millisecond,
		`"2m"`:     2 * time.Minute,
		`"1m0.5s"`: 60*time.Second + 500*time.Millisecond,
		// Sub-millisecond fractions are exact too.
		`"2.000000001s"`: 2*time.Second + 1,
		`"3.0000004s"`:   3*time.Second + 400,
		`"1.999999999s"`: 2*time.Second - 1,
	}
	for value, want := range cases {
		dir := writeConfigFile(t, "[ui]\noutput_bucket = "+value+"\n")
		got, err := LoadFrom(environment(map[string]string{"DECK_HOME": dir}), fakeHome)
		if err != nil || got.OutputBucket != want || got.File.OutputBucket != want {
			t.Errorf("output_bucket = %s gave %v / %v, %v; want exactly %v", value, got.OutputBucket, got.File.OutputBucket, err, want)
			continue
		}
		path := filepath.Join(t.TempDir(), "config.toml")
		if err := WriteConfigFile(path, got.File); err != nil {
			t.Fatal(err)
		}
		reread, err := loadConfigFile(path)
		if err != nil || reread.OutputBucket != want {
			t.Errorf("output_bucket %s written and re-read gave %v, %v; want exactly %v", value, reread.OutputBucket, err, want)
		}
	}
}

// A whole number of seconds is still written as the bare integer, and a
// fractional one as a quoted duration that re-reads to the same value.
func TestOutputBucketWriterKeepsWholeSecondsBareAndFractionsQuoted(t *testing.T) {
	for _, tc := range []struct {
		bucket time.Duration
		line   string
	}{
		{10 * time.Second, "output_bucket = 10"},
		{1500 * time.Millisecond, `output_bucket = "1.5s"`},
		{2250 * time.Millisecond, `output_bucket = "2.25s"`},
	} {
		path := filepath.Join(t.TempDir(), "config.toml")
		cfg, err := loadConfigFile(path)
		if err != nil {
			t.Fatal(err)
		}
		cfg.OutputBucket = tc.bucket
		if err := WriteConfigFile(path, cfg); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(path)
		if !strings.Contains(string(data), tc.line) {
			t.Errorf("bucket %v: config.toml = %q, want a line %q", tc.bucket, data, tc.line)
		}
		reread, err := loadConfigFile(path)
		if err != nil || reread.OutputBucket != tc.bucket {
			t.Errorf("bucket %v re-read as %v, %v", tc.bucket, reread.OutputBucket, err)
		}
	}
}

// Under the minimum stays an error: a fractional duration just below one
// second is refused, not rounded up to the minimum.
func TestOutputBucketJustBelowOneSecondIsRefused(t *testing.T) {
	for _, value := range []string{`"999ms"`, `"0.5s"`, `"-1500ms"`} {
		_, err := loadEventHookBody(t, "[ui]\noutput_bucket = "+value+"\n")
		if err == nil || !strings.Contains(err.Error(), "ui.output_bucket must be at least 1") {
			t.Errorf("value %s: err = %v, want \"ui.output_bucket must be at least 1\"", value, err)
		}
	}
}

// OutputBucketIndex is the one bucket rule: exact for fractional widths, and
// a width under the minimum never divides by zero.
func TestOutputBucketIndexUsesTheExactWidth(t *testing.T) {
	for _, tc := range []struct {
		ms    int64
		width time.Duration
		want  int64
	}{
		{3000, 1500 * time.Millisecond, 2},
		{4000, 1500 * time.Millisecond, 2},
		{4500, 1500 * time.Millisecond, 3},
		{109_999, 10 * time.Second, 10},
		{110_000, 10 * time.Second, 11},
		{7000, 2500 * time.Millisecond, 2},
		{8000, 2500 * time.Millisecond, 3},
		{5000, 0, 5},
	} {
		if got := OutputBucketIndex(tc.ms, tc.width); got != tc.want {
			t.Errorf("OutputBucketIndex(%d, %v) = %d, want %d", tc.ms, tc.width, got, tc.want)
		}
	}
}

// R306: a width with a sub-millisecond fraction keeps its exact boundaries.
// Rounding the width to whole milliseconds would move the boundary by up to
// one second per million buckets, which at a real Unix clock splits seconds
// that the exact width puts in one bucket (and merges seconds it separates).
func TestOutputBucketIndexKeepsSubMillisecondFractions(t *testing.T) {
	for _, tc := range []struct {
		width   time.Duration
		seconds []int64 // Unix seconds, each a window_activity value
		want    []int64 // floor(seconds*1e9 / width-in-ns)
	}{
		{2*time.Second + 1, []int64{1699999999, 1700000000, 1700000001, 1700000002, 1700000003}, []int64{849999999, 849999999, 850000000, 850000000, 850000001}},
		{3*time.Second + 400, []int64{1699999999, 1700000000, 1700000001, 1700000002, 1700000003}, []int64{566666590, 566666591, 566666591, 566666591, 566666592}},
		{2*time.Second - 1, []int64{1699999999, 1700000000, 1700000001, 1700000002}, []int64{849999999, 850000000, 850000000, 850000001}},
		{time.Second + 500, []int64{1700000000, 1700000001}, []int64{1699999150, 1699999151}},
	} {
		for i, sec := range tc.seconds {
			if got := OutputBucketIndex(sec*1000, tc.width); got != tc.want[i] {
				t.Errorf("OutputBucketIndex(%d s, %v) = %d, want %d", sec, tc.width, got, tc.want[i])
			}
		}
	}
	// The far end of the int64 millisecond range neither overflows nor
	// loses the fraction; a negative instant floors toward minus infinity.
	if got, want := OutputBucketIndex(1<<62, 2*time.Second+1), int64(2305843008060772); got != want {
		t.Errorf("OutputBucketIndex(2^62 ms, 2.000000001s) = %d, want %d", got, want)
	}
	if got, want := OutputBucketIndex(-(1<<62), 2*time.Second+1), int64(-2305843008060773); got != want {
		t.Errorf("OutputBucketIndex(-2^62 ms, 2.000000001s) = %d, want %d", got, want)
	}
	// Either side of the last millisecond whose nanoseconds fit in int64.
	limit := int64(math.MaxInt64) / int64(time.Millisecond)
	if got, want := OutputBucketIndex(limit, 2*time.Second+1), int64(4611686016); got != want {
		t.Errorf("OutputBucketIndex(limit, 2.000000001s) = %d, want %d", got, want)
	}
	if got, want := OutputBucketIndex(limit+1, 2*time.Second+1), int64(4611686016); got != want {
		t.Errorf("OutputBucketIndex(limit+1, 2.000000001s) = %d, want %d", got, want)
	}
	if got := OutputBucketIndex(-1, time.Second); got != -1 {
		t.Errorf("OutputBucketIndex(-1 ms, 1s) = %d, want -1", got)
	}
	if got := OutputBucketIndex(-2000, time.Second); got != -2 {
		t.Errorf("OutputBucketIndex(-2000 ms, 1s) = %d, want -2", got)
	}
}
