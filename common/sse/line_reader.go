package sse

import (
	"bufio"
	"bytes"
	"io"

	"github.com/Laisky/errors/v2"
)

const (
	// DefaultLineBufferSize defines the default buffer size for small SSE lines.
	DefaultLineBufferSize = 64 * 1024

	dataLinePrefix  = "data: "
	eventLinePrefix = "event: "
	commentPrefix   = ":"
)

var (
	dataPrefixBytes  = []byte("data:")
	eventPrefixBytes = []byte("event:")

	// ErrLineTooLong indicates a non-data SSE line exceeded the internal buffer.
	ErrLineTooLong = errors.New("sse line exceeded internal buffer")
)

// LineKind classifies a single SSE line.
type LineKind int

const (
	// LineKindBlank indicates an empty line.
	LineKindBlank LineKind = iota
	// LineKindComment indicates an SSE comment line that starts with ':'.
	LineKindComment
	// LineKindEvent indicates an SSE event line that starts with 'event:'.
	LineKindEvent
	// LineKindData indicates an SSE data line that starts with 'data:'.
	LineKindData
	// LineKindOther indicates any other line.
	LineKindOther
)

// Line represents one logical SSE line without the trailing newline characters.
type Line struct {
	Kind      LineKind
	Prefix    string
	Oversized bool
	Small     []byte
	Large     io.Reader
}

// Text returns the small in-memory line content as a string.
func (l Line) Text() string {
	return string(l.Small)
}

// LineReader reads SSE lines without relying on bufio.Scanner token limits.
type LineReader struct {
	reader      *bufio.Reader
	activeLarge *largeDataReader
}

// NewLineReader creates a LineReader for the provided reader and buffer size.
// It returns a reader that keeps small lines in memory and streams oversized
// data lines through Line.Large.
func NewLineReader(r io.Reader, bufSize int) *LineReader {
	if bufSize <= 0 {
		bufSize = DefaultLineBufferSize
	}

	return &LineReader{
		reader: bufio.NewReaderSize(r, bufSize),
	}
}

// Next reads the next logical SSE line. It returns io.EOF when the upstream
// reader is exhausted. If the previous oversized data line was not fully
// consumed, Next discards its remaining bytes before advancing.
func (r *LineReader) Next() (Line, error) {
	if r.activeLarge != nil {
		if err := r.activeLarge.Discard(); err != nil {
			return Line{}, errors.Wrap(err, "discard oversized SSE line")
		}
	}

	fragment, err := r.reader.ReadSlice('\n')
	switch {
	case err == nil:
		return classifyLine(normalizeSmallLine(fragment)), nil
	case errors.Is(err, bufio.ErrBufferFull):
		return r.buildLargeLine(fragment)
	case errors.Is(err, io.EOF):
		if len(fragment) == 0 {
			return Line{}, io.EOF
		}
		return classifyLine(normalizeSmallLine(fragment)), nil
	default:
		return Line{}, errors.Wrap(err, "read SSE line")
	}
}

// buildLargeLine constructs a streamed oversized data line from the initial fragment.
func (r *LineReader) buildLargeLine(fragment []byte) (Line, error) {
	trimmed := trimTrailingCarriageReturn(fragment)
	if !bytes.HasPrefix(trimmed, dataPrefixBytes) {
		return Line{}, errors.Wrap(ErrLineTooLong, "oversized non-data SSE line")
	}

	payload := trimLeftSpaces(trimmed[len(dataPrefixBytes):])
	large := newLargeDataReader(r, r.reader, payload)
	r.activeLarge = large

	return Line{
		Kind:      LineKindData,
		Prefix:    dataLinePrefix,
		Oversized: true,
		Large:     large,
	}, nil
}

// largeDataReader streams the payload portion of an oversized data line.
type largeDataReader struct {
	owner        *LineReader
	reader       *bufio.Reader
	pending      []byte
	finalPending bool
	finished     bool
}

// newLargeDataReader creates a largeDataReader with the initial payload fragment.
func newLargeDataReader(owner *LineReader, reader *bufio.Reader, initial []byte) *largeDataReader {
	return &largeDataReader{
		owner:   owner,
		reader:  reader,
		pending: cloneBytes(initial),
	}
}

// Read implements io.Reader for the oversized data payload.
func (r *largeDataReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	if r.finished {
		return 0, io.EOF
	}

	for len(r.pending) == 0 {
		if r.finalPending {
			r.finish()
			return 0, io.EOF
		}

		if err := r.fillPending(); err != nil {
			if errors.Is(err, io.EOF) {
				r.finish()
				return 0, io.EOF
			}

			r.finish()
			return 0, errors.Wrap(err, "fill pending sse buffer")
		}
	}

	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	if len(r.pending) == 0 && r.finalPending {
		r.finish()
	}

	return n, nil
}

// Discard drains any unread bytes from the oversized payload reader.
func (r *largeDataReader) Discard() error {
	if r.finished {
		return nil
	}

	if _, err := io.Copy(io.Discard, r); err != nil && !errors.Is(err, io.EOF) {
		return errors.Wrap(err, "discard oversized SSE payload")
	}

	return nil
}

// fillPending loads the next fragment of the oversized payload into memory.
func (r *largeDataReader) fillPending() error {
	fragment, err := r.reader.ReadSlice('\n')
	switch {
	case err == nil:
		trimmed := trimTrailingLineEnding(fragment)
		if len(trimmed) == 0 {
			r.finalPending = true
			return io.EOF
		}

		r.pending = cloneBytes(trimmed)
		r.finalPending = true
		return nil
	case errors.Is(err, bufio.ErrBufferFull):
		r.pending = cloneBytes(trimTrailingCarriageReturn(fragment))
		r.finalPending = false
		return nil
	case errors.Is(err, io.EOF):
		trimmed := trimTrailingCarriageReturn(fragment)
		if len(trimmed) == 0 {
			r.finalPending = true
			return io.EOF
		}

		r.pending = cloneBytes(trimmed)
		r.finalPending = true
		return nil
	default:
		return errors.Wrap(err, "read oversized SSE payload")
	}
}

// finish marks the oversized payload reader as complete and releases it from the owner.
func (r *largeDataReader) finish() {
	if r.finished {
		return
	}

	r.finished = true
	r.pending = nil
	if r.owner != nil && r.owner.activeLarge == r {
		r.owner.activeLarge = nil
	}
}

// classifyLine converts a normalized line into the exported Line structure.
func classifyLine(raw []byte) Line {
	line := Line{Small: cloneBytes(raw)}

	switch {
	case len(raw) == 0:
		line.Kind = LineKindBlank
	case raw[0] == ':':
		line.Kind = LineKindComment
		line.Prefix = commentPrefix
	case bytes.HasPrefix(raw, eventPrefixBytes):
		line.Kind = LineKindEvent
		line.Prefix = eventLinePrefix
	case bytes.HasPrefix(raw, dataPrefixBytes):
		line.Kind = LineKindData
		line.Prefix = dataLinePrefix
	default:
		line.Kind = LineKindOther
	}

	return line
}

// normalizeSmallLine removes the trailing CRLF or LF bytes from a small line.
func normalizeSmallLine(raw []byte) []byte {
	return cloneBytes(trimTrailingLineEnding(raw))
}

// trimTrailingLineEnding removes a single trailing LF and optional preceding CR.
func trimTrailingLineEnding(raw []byte) []byte {
	if len(raw) == 0 {
		return raw
	}

	if raw[len(raw)-1] == '\n' {
		raw = raw[:len(raw)-1]
	}

	return trimTrailingCarriageReturn(raw)
}

// trimTrailingCarriageReturn removes a trailing CR from a line fragment.
func trimTrailingCarriageReturn(raw []byte) []byte {
	if len(raw) > 0 && raw[len(raw)-1] == '\r' {
		return raw[:len(raw)-1]
	}

	return raw
}

// trimLeftSpaces removes only ASCII spaces from the beginning of a data payload.
func trimLeftSpaces(raw []byte) []byte {
	for len(raw) > 0 && raw[0] == ' ' {
		raw = raw[1:]
	}

	return raw
}

// cloneBytes copies a byte slice so returned lines remain stable across reads.
func cloneBytes(raw []byte) []byte {
	if len(raw) == 0 {
		return nil
	}

	dup := make([]byte, len(raw))
	copy(dup, raw)
	return dup
}
