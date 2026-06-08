package input

// Interpreter translates the input byte stream into forwarded bytes + actions.
// It buffers a partial escape sequence between Feed calls. Not safe for
// concurrent use; the event loop feeds it serially.
type Interpreter struct {
	mode Mode
	buf  []byte // pending bytes (a possibly-incomplete escape sequence)
}

// New returns an Interpreter in normal mode.
func New() *Interpreter { return &Interpreter{mode: ModeNormal} }

// Mode reports the current mode.
func (it *Interpreter) Mode() Mode { return it.mode }

const esc = 0x1b

// Feed consumes p and returns bytes to forward plus actions. A trailing
// incomplete escape sequence is retained for the next call.
func (it *Interpreter) Feed(p []byte) Result {
	it.buf = append(it.buf, p...)
	var res Result
	for len(it.buf) > 0 {
		tok, n, complete := nextToken(it.buf)
		if !complete {
			break // wait for more bytes
		}
		it.buf = it.buf[n:]
		it.handle(tok, &res)
	}
	return res
}

// token is one decoded unit of input.
type token struct {
	csi    bool   // true if this is an ESC[ ... sequence
	bytes  []byte // the raw bytes of the token (the whole CSI, or one plain byte)
	final  byte   // CSI final byte (e.g. '~', 'A', 'M', 'm'); 0 for plain
	params string // CSI parameter bytes between '[' and final (e.g. "5;2", "<64;1;1")
}

// nextToken decodes the next token from buf. complete=false means buf holds an
// incomplete sequence and the caller should wait for more bytes.
func nextToken(buf []byte) (tok token, consumed int, complete bool) {
	if buf[0] != esc {
		return token{bytes: buf[:1]}, 1, true // plain byte
	}
	// ESC...
	if len(buf) < 2 {
		return token{}, 0, false // lone ESC: wait (could be CSI lead)
	}
	if buf[1] != '[' {
		// ESC followed by something else: treat the ESC as a standalone byte
		// (Escape key). The following byte(s) are decoded on the next loop.
		return token{bytes: buf[:1]}, 1, true
	}
	// CSI: ESC [ params... final(0x40-0x7e)
	for i := 2; i < len(buf); i++ {
		b := buf[i]
		if b >= 0x40 && b <= 0x7e {
			return token{csi: true, bytes: buf[:i+1], final: b, params: string(buf[2:i])}, i + 1, true
		}
	}
	return token{}, 0, false // CSI not yet terminated
}

// handle applies one token to the result, possibly changing mode.
func (it *Interpreter) handle(tok token, res *Result) {
	if it.mode == ModeNormal {
		it.handleNormal(tok, res)
		return
	}
	it.handleCopy(tok, res)
}

func (it *Interpreter) handleNormal(tok token, res *Result) {
	if tok.csi {
		switch {
		case tok.final == '~' && tok.params == "5;2": // Shift+PageUp
			it.mode = ModeCopy
			res.Actions = append(res.Actions, EnterCopyMode, ScrollPageUp)
			return
		case tok.final == 'M' && isWheelUp(tok.params): // mouse wheel up
			it.mode = ModeCopy
			res.Actions = append(res.Actions, EnterCopyMode, ScrollLineUp)
			return
		case tok.final == 'M' || tok.final == 'm': // other mouse events: swallow
			return
		}
		res.Forward = append(res.Forward, tok.bytes...) // unrecognized CSI: forward
		return
	}
	res.Forward = append(res.Forward, tok.bytes...) // plain byte: forward
}

// TEMP: real implementations land in Tasks 2 (isWheelUp) and 3 (handleCopy).
func isWheelUp(string) bool                               { return false }
func (it *Interpreter) handleCopy(tok token, res *Result) { /* Task 3 */ }
