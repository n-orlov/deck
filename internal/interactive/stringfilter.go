package interactive

import "sync"

// stringFilter is the one pre-filter in front of the emulator (R209, issue
// #69). The vt parser (charmbracelet/x/ansi) is byte-based: inside an
// escape string it treats the C1 terminator 0x9C as ST, so a UTF-8
// character whose encoding carries a byte in 0x80-0x9F -- the "✳" (E2 9C
// B3) Claude puts in its OSC window title -- ends the string early and the
// rest of the title prints at the cursor. The pane never needs a string's
// payload, so inside an OSC, DCS, SOS, PM or APC string the filter drops the
// bytes that would end it early and forwards the rest. The vt tables carry
// a payload differently per kind: OSC and the DCS passthrough accept
// 0x80-0xFF, so there it drops the C1 range 0x80-0x9F; SOS, PM and APC (and
// a DCS header, before its final byte) accept 0x00-0x7F only, so a non-ASCII
// byte would leave the string early too, and every byte >= 0x80 is dropped.
//
// Outside a string it forwards every byte unchanged and never buffers:
// filter returns its argument itself when nothing was dropped, and only a
// chunk that contained a dropped byte is copied. The recognizer carries
// its state across calls, so a read split anywhere (between E2 and 9C B3
// included) yields the same bytes as an unsplit one.
//
// Strings begin at ESC ] / P / X / ^ / _. They end where the emulator ends
// them: BEL (OSC only) and ESC, which the parser then reads as the start of
// an escape sequence (ESC \ being ST), plus CAN and SUB, which the parser
// treats as an abort anywhere.
type stringFilter struct {
	mu    sync.Mutex
	state filterState
}

type filterState uint8

const (
	filterGround filterState = iota
	// filterEscape follows an ESC, whether it came from the ground or ended
	// a string: the next byte decides whether a new string begins.
	filterEscape
	// filterEscapeIntermediate follows ESC and an intermediate byte
	// (0x20-0x2F), so a following ] or P is a final byte, not a string.
	filterEscapeIntermediate
	// filterOSC is inside ESC ] ...; it ends on BEL as well as ESC.
	filterOSC
	// filterText is inside an SOS, PM or APC string.
	filterText
	// The DCS header states run from ESC P to the final byte; filterDCSData
	// is the passthrough after it.
	filterDCSEntry
	filterDCSParam
	filterDCSIntermediate
	filterDCSData
)

const (
	byteBEL = 0x07
	byteCAN = 0x18
	byteSUB = 0x1A
	byteESC = 0x1B
)

// drops reports whether b is removed in the current state.
func (f *stringFilter) drops(b byte) bool {
	switch f.state {
	case filterOSC, filterDCSData:
		return b >= 0x80 && b <= 0x9F
	case filterText, filterDCSEntry, filterDCSParam, filterDCSIntermediate:
		return b >= 0x80
	}
	return false
}

// filter returns p without the bytes that fall inside an escape string and
// would end it early. The result is p itself when no byte was dropped,
// otherwise a fresh slice; p is never modified.
func (f *stringFilter) filter(p []byte) []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []byte
	dropped := false
	for i, b := range p {
		if f.drops(b) {
			if !dropped {
				dropped = true
				out = make([]byte, i, len(p)-1)
				copy(out, p[:i])
			}
			continue
		}
		f.advance(b)
		if dropped {
			out = append(out, b)
		}
	}
	if !dropped {
		return p
	}
	return out
}

// advance moves the recognizer over one forwarded byte.
func (f *stringFilter) advance(b byte) {
	if isAbort(b) {
		f.state = filterGround
		return
	}
	switch f.state {
	case filterGround:
		if b == byteESC {
			f.state = filterEscape
		}
	case filterEscape:
		f.advanceEscape(b)
	case filterEscapeIntermediate:
		f.advanceEscapeIntermediate(b)
	case filterDCSEntry:
		f.advanceDCSEntry(b)
	case filterDCSParam:
		f.advanceDCSParam(b)
	case filterDCSIntermediate:
		f.advanceDCSIntermediate(b)
	default: // filterOSC, filterText, filterDCSData
		f.advanceString(b)
	}
}

// isAbort reports CAN and SUB, which the parser treats as an abort in every
// state.
func isAbort(b byte) bool { return b == byteCAN || b == byteSUB }

// advanceString follows a payload: ESC leaves it for an escape sequence, and
// BEL ends it when it is an OSC.
func (f *stringFilter) advanceString(b byte) {
	switch {
	case b == byteESC:
		f.state = filterEscape
	case b == byteBEL && f.state == filterOSC:
		f.state = filterGround
	}
}

func (f *stringFilter) advanceEscapeIntermediate(b byte) {
	switch {
	case b == byteESC:
		f.state = filterEscape
	case b >= 0x30 && b <= 0x7E:
		f.state = filterGround
	}
}

// The three DCS header states follow the vt table and reach the passthrough on
// the final byte. As in the table, an ESC right after ESC P is payload, not
// the start of an escape sequence.
func (f *stringFilter) advanceDCSEntry(b byte) {
	switch {
	case b == byteESC || (b >= 0x08 && b <= 0x0D) || (b >= 0x40 && b <= 0x7E):
		f.state = filterDCSData
	case b >= 0x20 && b <= 0x2F:
		f.state = filterDCSIntermediate
	case b >= 0x30 && b <= 0x3F:
		f.state = filterDCSParam
	}
}

func (f *stringFilter) advanceDCSParam(b byte) {
	switch {
	case b == byteESC:
		f.state = filterEscape
	case b >= 0x40 && b <= 0x7E:
		f.state = filterDCSData
	case b >= 0x20 && b <= 0x2F:
		f.state = filterDCSIntermediate
	}
}

func (f *stringFilter) advanceDCSIntermediate(b byte) {
	switch {
	case b == byteESC:
		f.state = filterEscape
	case b >= 0x30 && b <= 0x7E:
		f.state = filterDCSData
	}
}

// advanceEscape handles the byte after an ESC.
func (f *stringFilter) advanceEscape(b byte) {
	switch b {
	case ']':
		f.state = filterOSC
	case 'P':
		f.state = filterDCSEntry
	case 'X', '^', '_':
		f.state = filterText
	case byteESC:
		// stays in filterEscape
	default:
		switch {
		case b >= 0x20 && b <= 0x2F:
			f.state = filterEscapeIntermediate
		case b >= 0x30 && b <= 0x7E:
			f.state = filterGround
		}
		// other C0 controls and DEL keep the parser in the escape state
	}
}
