package main

import (
	"fmt"
	"io"
)

// say, sayf and sayln write operator-facing text (usage, diagnostics, the
// progress line) to w. A failed write to a terminal or pipe the operator has
// already closed has no recovery path -- there is nowhere left to report it --
// so the error is dropped here, in one place, rather than at every call site.
// The exit status, not the write, is what a caller of the command checks.
func say(w io.Writer, a ...any) { _, _ = fmt.Fprint(w, a...) }

func sayf(w io.Writer, format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }

func sayln(w io.Writer, a ...any) { _, _ = fmt.Fprintln(w, a...) }
