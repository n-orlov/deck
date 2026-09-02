package tmux

import (
	"context"
	"testing"
	"time"
)

// TestEncodeIsizeGeometryFixedForm proves EncodeIsizeGeometry's exact wire
// shape for both WindowSizeSet cases named by R100: a set window-size
// value fills the final field verbatim, and an unset one leaves it empty
// but keeps the trailing `:`.
func TestEncodeIsizeGeometryFixedForm(t *testing.T) {
	set := EncodeIsizeGeometry(WindowGeometry{Width: 80, Height: 24, WindowSizeSet: true, WindowSizeValue: "manual"})
	if set != "80x24:manual" {
		t.Fatalf("encode set window-size = %q, want %q", set, "80x24:manual")
	}
	unset := EncodeIsizeGeometry(WindowGeometry{Width: 80, Height: 24, WindowSizeSet: false, WindowSizeValue: "should-be-ignored"})
	if unset != "80x24:" {
		t.Fatalf("encode unset window-size = %q, want %q (WindowSizeValue must be ignored when WindowSizeSet is false)", unset, "80x24:")
	}
}

// TestParseIsizeGeometryRoundTripsSetAndUnsetWindowSize proves the
// encode/parse pair round-trips byte-exactly for both WindowSizeSet
// states R100 names.
func TestParseIsizeGeometryRoundTripsSetAndUnsetWindowSize(t *testing.T) {
	cases := []WindowGeometry{
		{Width: 80, Height: 24, WindowSizeSet: true, WindowSizeValue: "manual"},
		{Width: 200, Height: 55, WindowSizeSet: true, WindowSizeValue: "latest"},
		{Width: 80, Height: 24, WindowSizeSet: false},
	}
	for _, want := range cases {
		encoded := EncodeIsizeGeometry(want)
		got, ok := ParseIsizeGeometry(encoded)
		if !ok {
			t.Fatalf("ParseIsizeGeometry(%q) ok = false, want true (round-trip of %+v)", encoded, want)
		}
		if got != want {
			t.Fatalf("ParseIsizeGeometry(%q) = %+v, want %+v", encoded, got, want)
		}
	}
}

// TestParseIsizeGeometryUnparseableValuesAreAbsent proves R100's explicit
// fallback: "a value that does not parse is treated as absent" -- ok=false
// with a zero WindowGeometry, never an error and never a partial parse,
// for every shape of malformed value this option could plausibly see.
func TestParseIsizeGeometryUnparseableValuesAreAbsent(t *testing.T) {
	unparseable := []string{
		"",
		"garbage",
		"80x24",            // missing the ':' field separator entirely
		"80:manual",        // missing the 'x' dimension separator
		"eightyx24:manual", // non-integer width
		"x24:manual",       // empty width
		"80x:manual",       // empty height
	}
	for _, value := range unparseable {
		got, ok := ParseIsizeGeometry(value)
		if ok {
			t.Fatalf("ParseIsizeGeometry(%q) ok = true (parsed as %+v), want false (unparseable, treated as absent)", value, got)
		}
		if got != (WindowGeometry{}) {
			t.Fatalf("ParseIsizeGeometry(%q) returned non-zero geometry %+v alongside ok=false", value, got)
		}
	}
}

// TestParseIsizeGeometryToleratesColonsInsideTheSizeField proves the size
// field is only ever split on the FIRST ':' after the dimensions --
// window-size values never legitimately contain a colon, but the parser
// deliberately keeps the rest of the string verbatim as the size field
// rather than erroring, so a foreign writer's stray colon degrades to an
// odd-but-parsed value instead of an absent one.
func TestParseIsizeGeometryToleratesColonsInsideTheSizeField(t *testing.T) {
	got, ok := ParseIsizeGeometry("80x24:manual:extra")
	if !ok {
		t.Fatalf("ParseIsizeGeometry with an extra colon in the size field: ok = false, want true")
	}
	want := WindowGeometry{Width: 80, Height: 24, WindowSizeSet: true, WindowSizeValue: "manual:extra"}
	if got != want {
		t.Fatalf("ParseIsizeGeometry with an extra colon in the size field = %+v, want %+v", got, want)
	}
}

// TestWindowIsizeGeometryHelpersRoundTripAgainstRealTmux proves the
// window-scoped read/write/unset helpers against a real tmux server on a
// private socket: unset before anything ever writes it (the "@"-prefixed
// user option's "invalid option" shape, same as OwnershipOption); a
// write-then-read round-trips a set window-size value byte-exactly; a
// second write-then-read round-trips an unset window-size value (empty
// final field) byte-exactly; and unset returns the option to its original
// absent state.
func TestWindowIsizeGeometryHelpersRoundTripAgainstRealTmux(t *testing.T) {
	socket := geometrySocket("isize")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 24)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}
	ctx := context.Background()

	if _, ok, err := client.readWindowIsizeGeometry(ctx, "s0"); err != nil || ok {
		t.Fatalf("read before anything ever wrote %s: ok=%v err=%v, want ok=false err=nil", IsizeGeometryOption, ok, err)
	}

	withWindowSize := WindowGeometry{Width: 80, Height: 24, WindowSizeSet: true, WindowSizeValue: "manual"}
	if err := client.writeWindowIsizeGeometry(ctx, "s0", withWindowSize); err != nil {
		t.Fatalf("write %s (set window-size): %v", IsizeGeometryOption, err)
	}
	got, ok, err := client.readWindowIsizeGeometry(ctx, "s0")
	if err != nil || !ok {
		t.Fatalf("read %s after writing a set window-size: ok=%v err=%v, want ok=true err=nil", IsizeGeometryOption, ok, err)
	}
	if got != withWindowSize {
		t.Fatalf("read %s after writing a set window-size = %+v, want %+v", IsizeGeometryOption, got, withWindowSize)
	}

	withoutWindowSize := WindowGeometry{Width: 200, Height: 55, WindowSizeSet: false}
	if err := client.writeWindowIsizeGeometry(ctx, "s0", withoutWindowSize); err != nil {
		t.Fatalf("write %s (unset window-size): %v", IsizeGeometryOption, err)
	}
	got, ok, err = client.readWindowIsizeGeometry(ctx, "s0")
	if err != nil || !ok {
		t.Fatalf("read %s after writing an unset window-size: ok=%v err=%v, want ok=true err=nil", IsizeGeometryOption, ok, err)
	}
	if got != withoutWindowSize {
		t.Fatalf("read %s after writing an unset window-size = %+v, want %+v", IsizeGeometryOption, got, withoutWindowSize)
	}

	if err := client.unsetWindowIsizeGeometry(ctx, "s0"); err != nil {
		t.Fatalf("unset %s: %v", IsizeGeometryOption, err)
	}
	if _, ok, err := client.readWindowIsizeGeometry(ctx, "s0"); err != nil || ok {
		t.Fatalf("read %s after unset: ok=%v err=%v, want ok=false err=nil", IsizeGeometryOption, ok, err)
	}
}

// TestWindowIsizeGeometryHelperReportsUnparseableAsAbsent proves the
// window-scoped read helper applies R100's "unparseable is treated as
// absent" rule too, not just the pure ParseIsizeGeometry function: a raw
// tmux write of a value the encoding never produces still reads back as
// ok=false, err=nil -- never a parse error surfaced to the caller.
func TestWindowIsizeGeometryHelperReportsUnparseableAsAbsent(t *testing.T) {
	socket := geometrySocket("isize-unparseable")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 24)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}
	ctx := context.Background()

	runTmux(t, socket, "set-option", "-w", "-t", "s0", IsizeGeometryOption, "not-a-valid-encoding")

	got, ok, err := client.readWindowIsizeGeometry(ctx, "s0")
	if err != nil {
		t.Fatalf("read %s holding an unparseable value: err = %v, want nil", IsizeGeometryOption, err)
	}
	if ok {
		t.Fatalf("read %s holding an unparseable value: ok = true (parsed as %+v), want false (treated as absent)", IsizeGeometryOption, got)
	}
}
