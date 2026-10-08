package notify

import (
	"sync"
	"unicode/utf8"
)

// tailBuffer is an io.Writer that keeps only the last limit bytes written. It
// always accepts everything, so a chatty script never blocks on a full pipe
// and memory stays bounded however much it prints.
type tailBuffer struct {
	mu      sync.Mutex
	limit   int
	buf     []byte
	dropped bool
}

func newTailBuffer(limit int) *tailBuffer {
	return &tailBuffer{limit: limit}
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if over := len(t.buf) - t.limit; over > 0 {
		t.buf = append(t.buf[:0], t.buf[over:]...)
		t.dropped = true
	}
	return len(p), nil
}

// tail returns the kept bytes, trimmed forward to a rune start, and whether
// anything earlier was dropped.
func (t *tailBuffer) tail() (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	b := t.buf
	for len(b) > 0 && !utf8.RuneStart(b[0]) {
		b = b[1:]
	}
	return string(b), t.dropped
}
