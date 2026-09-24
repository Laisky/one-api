package sse

import (
	"bytes"

	"github.com/Laisky/errors/v2"
)

// NextBuffered returns a complete line already buffered in memory without reading
// the upstream. The boolean is false when Next may need I/O or when an oversized
// payload is still owned by the caller. Like Next, calls must be sequential.
func (r *LineReader) NextBuffered() (Line, bool, error) {
	if r.activeLarge != nil || r.reader.Buffered() == 0 {
		return Line{}, false, nil
	}
	buffered, err := r.reader.Peek(r.reader.Buffered())
	if err != nil {
		return Line{}, true, errors.Wrap(err, "peek buffered SSE data")
	}
	if bytes.IndexByte(buffered, '\n') < 0 {
		return Line{}, false, nil
	}
	// The newline is already present: ReadSlice cannot refill the buffer. The
	// classifier makes the sole owned copy, so future reads cannot mutate it.
	fragment, err := r.reader.ReadSlice('\n')
	if err != nil {
		return Line{}, true, errors.Wrap(err, "read buffered SSE line")
	}
	return classifyLine(trimTrailingLineEnding(fragment)), true, nil
}
