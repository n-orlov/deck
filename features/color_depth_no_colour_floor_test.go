package features

import (
	"context"
	"testing"
	"time"
)

// TestClientFrameHasNoColourAnywhereCanFail is the negative proof for
// clientFrameHasNoColourAnywhere (operator steer, 21/22 Aug 2026 -- see
// docs/reports/phase2b2-findings.md): the walk that backs requirement
// 3/31's NO_COLOR-by-glyphs-alone assertion must be able to fail when
// pointed at a client that actually has colour turned on, not merely pass
// by examining a grid it never really inspected (an unsized emulator, or
// one whose cells never got populated). This mirrors
// features/emulator_placement_test.go's TestCellAttributeAssertionsCanFail
// precedent, but for THIS specific walk: the commit that first added the
// floor and the direct Style.Fg/Bg checks proved they could fail against a
// throwaway scenario that was never committed, which is exactly the gap
// this test closes -- the ability to fail now lives in the tree, not only
// in a commit message.
func TestClientFrameHasNoColourAnywhereCanFail(t *testing.T) {
	binary := buildDeckBinary(t)
	h, err := newScenarioHarness(binary)
	if err != nil {
		t.Fatalf("create scenario harness: %v", err)
	}
	t.Cleanup(func() {
		if err := h.Close(); err != nil {
			t.Errorf("scenario harness teardown: %v", err)
		}
	})

	ctx := context.WithValue(context.Background(), scenarioHarnessKey{}, h)
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	// "NO_COLOR=" is the harness's own documented sentinel for lifting its
	// NO_COLOR=1 default (see startNamedClientWithColour in
	// cell_attributes_test.go) -- this client renders with real colour on,
	// the exact opposite of what clientFrameHasNoColourAnywhere requires.
	client, err := h.StartNamedClient(ctx, "floor-proof", "NO_COLOR=")
	if err != nil {
		t.Fatalf("start colour-enabled client: %v", err)
	}
	if err := client.WaitForFrame(ctx, false, "deck - sessions"); err != nil {
		t.Fatalf("client did not render its main list: %v", err)
	}

	if err := clientFrameHasNoColourAnywhere(ctx, "floor-proof"); err == nil {
		t.Fatal("want clientFrameHasNoColourAnywhere to report colour against a colour-enabled client, got nil")
	} else {
		t.Logf("clientFrameHasNoColourAnywhere correctly failed against a colour-enabled client: %v", err)
	}

	// Exit the client the same way clientExitsCleanly does before the
	// harness's own strict Close checks for a surviving process.
	if err := client.Send("q"); err != nil {
		t.Fatalf("send quit key: %v", err)
	}
	if err := client.Stop(5 * time.Second); err != nil {
		t.Fatalf("deck client did not exit cleanly: %v", err)
	}
}
