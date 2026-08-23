//go:build deckinputcount

package main

import (
	"os"
	"sync/atomic"

	tea "github.com/charmbracelet/bubbletea"
)

// rawByteCountFileEnvironment names the file this build records the running
// total of bytes bubbletea's own cancel-reader consumes from stdin into --
// overwritten (not appended) after every successful Read, the same
// write-to-temp-then-rename discipline writeInputCount already uses. It is
// deliberately a SEPARATE file from DECK_INPUT_COUNT_FILE (inputcount_hook.go):
// that counter advances only after bubbletea has parsed a byte stream into a
// tea.KeyMsg and dispatched it into deck's own Update loop, while this one
// advances the instant bubbletea's reader (github.com/muesli/cancelreader,
// wrapped around this file via tea.WithInput) pulls the byte off the pty --
// strictly BEFORE any key-sequence parsing, coalescing or paste-bracket
// handling bubbletea or deck perform on it. Comparing the two lets I-1's
// investigation place a drop either at or below "the kernel delivered the
// byte to this process's stdin fd" (this file) versus "bubbletea decoded and
// dispatched it" (inputcount_hook.go) without patching bubbletea itself --
// tea.WithInput is a supported program option, not a fork.
const rawByteCountFileEnvironment = "DECK_RAW_BYTE_COUNT_FILE"

// rawByteCountingProgramOptions is compiled in only when a test explicitly
// builds `-tags deckinputcount` (see rawbytecount_default.go for the no-op
// every other build sees), and even then returns nil -- changing nothing
// about how tea.NewProgram picks its input -- unless
// DECK_RAW_BYTE_COUNT_FILE is set, which no documented deck control ever
// sets on a user's behalf.
func rawByteCountingProgramOptions() []tea.ProgramOption {
	path := os.Getenv(rawByteCountFileEnvironment)
	if path == "" {
		return nil
	}
	return []tea.ProgramOption{tea.WithInput(&rawByteCountingStdin{f: os.Stdin, path: path})}
}

// rawByteCountingStdin wraps os.Stdin so it still satisfies bubbletea's
// term.File requirement (io.ReadWriteCloser plus Fd()) -- required so
// bubbletea still recognises the input as a terminal, still calls
// term.MakeRaw on the real fd via Fd(), and behaves exactly as it would with
// os.Stdin passed directly -- while counting every byte its Read method
// hands back to the caller (bubbletea's cancelreader, one layer below key
// parsing).
type rawByteCountingStdin struct {
	f     *os.File
	path  string
	total int64
}

func (r *rawByteCountingStdin) Read(p []byte) (int, error) {
	n, err := r.f.Read(p)
	if n > 0 {
		total := atomic.AddInt64(&r.total, int64(n))
		writeInputCount(r.path, total)
	}
	return n, err
}

func (r *rawByteCountingStdin) Write(p []byte) (int, error) { return r.f.Write(p) }
func (r *rawByteCountingStdin) Close() error                { return r.f.Close() }
func (r *rawByteCountingStdin) Fd() uintptr                 { return r.f.Fd() }
