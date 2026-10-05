package tmux

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestReadFifoOnceTellsDataFromEmptyFromNoWriter(t *testing.T) {
	path, fd := newWaitTestFifo(t)
	buf := make([]byte, 64)

	// No writer has connected yet: not settled, keep polling.
	if data, done, err := readFifoOnce(fd, buf); done || data != nil || err != nil {
		t.Fatalf("no writer: readFifoOnce = %q, %v, %v; want not done", data, done, err)
	}
	writer, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = writer.Close() }()
	// A writer exists but has written nothing: settled, with no data.
	if data, done, err := readFifoOnce(fd, buf); !done || data != nil || err != nil {
		t.Fatalf("idle writer: readFifoOnce = %q, %v, %v; want done with no data", data, done, err)
	}
	if _, err := writer.WriteString("hello"); err != nil {
		t.Fatal(err)
	}
	if data, done, err := readFifoOnce(fd, buf); !done || string(data) != "hello" || err != nil {
		t.Fatalf("writer with data: readFifoOnce = %q, %v, %v; want done with hello", data, done, err)
	}
}

func TestReadFifoOnceReportsARealReadError(t *testing.T) {
	_, fd := newWaitTestFifo(t)
	closed, err := unix.Dup(fd)
	if err != nil {
		t.Fatal(err)
	}
	if err := unix.Close(closed); err != nil {
		t.Fatal(err)
	}
	if data, done, err := readFifoOnce(closed, make([]byte, 8)); !done || data != nil || !errors.Is(err, unix.EBADF) {
		t.Fatalf("closed descriptor: readFifoOnce = %q, %v, %v; want done with EBADF", data, done, err)
	}
}

func TestProbeFifoArmingNamesWhyTheWaitEnds(t *testing.T) {
	ctx := context.Background()
	if err := probeFifoArming(ctx, func(context.Context) (bool, error) { return true, nil }, time.Second); err != nil {
		t.Fatalf("armed pipe: probeFifoArming = %v, want nil", err)
	}
	if err := probeFifoArming(ctx, func(context.Context) (bool, error) { return false, nil }, time.Second); err == nil || !strings.Contains(err.Error(), "no longer armed") {
		t.Fatalf("unarmed pipe: probeFifoArming = %v, want the no-longer-armed error", err)
	}
	probeFailure := errors.New("tmux went away")
	if err := probeFifoArming(ctx, func(context.Context) (bool, error) { return false, probeFailure }, time.Second); !errors.Is(err, probeFailure) || !strings.Contains(err.Error(), "probing whether pipe-pane is still armed failed") {
		t.Fatalf("failing probe: probeFifoArming = %v, want it to wrap the probe failure", err)
	}
}

func TestWaitForFifoWriterReturnsDataAlreadyWritten(t *testing.T) {
	path, fd := newWaitTestFifo(t)
	writer, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = writer.Close() }()
	if _, err := writer.WriteString("early"); err != nil {
		t.Fatal(err)
	}
	data, err := waitForFifoWriter(context.Background(), fd, nil, time.Second, time.Second)
	if err != nil || string(data) != "early" {
		t.Fatalf("waitForFifoWriter = %q, %v; want the bytes already written", data, err)
	}
}

func TestWaitForFifoWriterTimesOutWhenNoWriterEverConnects(t *testing.T) {
	_, fd := newWaitTestFifo(t)
	_, err := waitForFifoWriter(context.Background(), fd, func(context.Context) (bool, error) { return true, nil }, 5*time.Millisecond, 30*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "timed out after") {
		t.Fatalf("waitForFifoWriter = %v, want the timeout error", err)
	}
}

func TestWaitForFifoWriterSurfacesAProbeFailureAtTheGraceMark(t *testing.T) {
	_, fd := newWaitTestFifo(t)
	probeFailure := errors.New("tmux went away")
	_, err := waitForFifoWriter(context.Background(), fd, func(context.Context) (bool, error) { return false, probeFailure }, 5*time.Millisecond, time.Second)
	if !errors.Is(err, probeFailure) {
		t.Fatalf("waitForFifoWriter = %v, want it to wrap the probe failure", err)
	}
}
