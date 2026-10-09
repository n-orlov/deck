package features

import (
	"context"
	"testing"
	"time"
)

// A resize to the size the terminal already has sends deck no SIGWINCH, so
// ResizeAndAwaitRender must return at once instead of waiting for a repaint
// that nothing owes. The child here prints nothing at all, so a wait for the
// render marker could only run out its resizeAwaitTimeout.
func TestResizeAndAwaitRenderToTheCurrentSizeDoesNotWaitForARepaint(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	driver, err := startScreenDriver(ctx, "sleep", nil, "", []string{"30"}, 100, 38)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = driver.Stop(time.Second) }()

	started := time.Now()
	if err := driver.ResizeAndAwaitRender(ctx, 100, 38); err != nil {
		t.Fatalf("resize to the current size = %v, want nil", err)
	}
	if elapsed := time.Since(started); elapsed >= resizeAwaitTimeout/2 {
		t.Fatalf("resize to the current size took %s: it waited for a repaint", elapsed)
	}
	if cols, rows := driver.GridSize(); cols != 100 || rows != 38 {
		t.Fatalf("grid = %dx%d, want 100x38", cols, rows)
	}
}
