package sse

import (
	"io"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// joinedEOFTerminalReader delivers one real terminal error, then ordinary EOF.
// Returning the mixed error only once prevents a later Read from hiding an erased cause.
type joinedEOFTerminalReader struct {
	prefix   *strings.Reader
	terminal error
	returned bool
}

// Read drains payload bytes and returns the selected error unchanged exactly once.
func (r *joinedEOFTerminalReader) Read(p []byte) (int, error) {
	if r.prefix.Len() > 0 {
		return r.prefix.Read(p)
	}
	if !r.returned {
		r.returned = true
		return 0, r.terminal
	}
	return 0, io.EOF
}

// TestLineReaderJoinedEOFPreservesIndependentCause exercises ordinary and final-fragment lines.
func TestLineReaderJoinedEOFPreservesIndependentCause(t *testing.T) {
	independent := errors.New("synthetic independent SSE transport failure")
	for _, cause := range []struct {
		name string
		err  error
	}{
		{"unexpected_eof", io.ErrUnexpectedEOF},
		{"independent_error", independent},
	} {
		for _, terminated := range []bool{false, true} {
			name := cause.name + "/unterminated_fragment"
			wire := "data: hello"
			if terminated {
				name = cause.name + "/complete_line"
				wire += "\n"
			}
			t.Run(name, func(t *testing.T) {
				source := &joinedEOFTerminalReader{prefix: strings.NewReader(wire), terminal: errors.Join(io.EOF, cause.err)}
				reader := NewLineReader(source, 64)
				line, err := reader.Next()
				if terminated {
					require.NoError(t, err)
					require.Equal(t, "data: hello", line.Text())
					_, err = reader.Next()
				} else if err == nil {
					// Either returning the partial line first or rejecting it immediately is
					// acceptable, but the independent terminal failure must still be observable.
					require.Equal(t, "data: hello", line.Text())
					_, err = reader.Next()
				}
				require.ErrorIs(t, err, cause.err, "a transport failure joined with EOF must not become clean EOF")
				require.True(t, source.returned, "the fixture's selected terminal error must actually reach the reader")
			})
		}
	}
}

// TestLineReaderJoinedEOFPureControls preserves wrapped and duplicate pure EOF behavior.
func TestLineReaderJoinedEOFPureControls(t *testing.T) {
	for _, terminal := range []struct {
		name string
		err  error
	}{
		{"wrapped_eof", errors.Wrap(io.EOF, "synthetic wrapper")},
		{"duplicate_eof", errors.Join(io.EOF, io.EOF)},
	} {
		for _, terminated := range []bool{false, true} {
			name := terminal.name + "/unterminated_fragment"
			wire := "data: hello"
			if terminated {
				name = terminal.name + "/complete_line"
				wire += "\n"
			}
			t.Run(name, func(t *testing.T) {
				reader := NewLineReader(&joinedEOFTerminalReader{prefix: strings.NewReader(wire), terminal: terminal.err}, 64)
				line, err := reader.Next()
				require.NoError(t, err)
				require.Equal(t, "data: hello", line.Text())
				_, err = reader.Next()
				require.ErrorIs(t, err, io.EOF)
			})
		}
	}
}

// TestLineReaderOversizedJoinedEOFPreservesIndependentCause exercises public Read and Next-discard paths.
func TestLineReaderOversizedJoinedEOFPreservesIndependentCause(t *testing.T) {
	independent := errors.New("synthetic independent oversized SSE failure")
	for _, cause := range []struct {
		name string
		err  error
	}{
		{"unexpected_eof", io.ErrUnexpectedEOF},
		{"independent_error", independent},
	} {
		for _, discard := range []bool{false, true} {
			mode := "read_payload"
			if discard {
				mode = "discard_via_next"
			}
			t.Run(cause.name+"/"+mode, func(t *testing.T) {
				payload := strings.Repeat("x", 513)
				source := &joinedEOFTerminalReader{prefix: strings.NewReader("data: " + payload), terminal: errors.Join(io.EOF, cause.err)}
				reader := NewLineReader(source, 32)
				line, err := reader.Next()
				require.NoError(t, err)
				require.True(t, line.Oversized, "fixture must reach the oversized streaming reader")
				if discard {
					// This invokes the normal public cleanup path without implementation helpers.
					_, err = reader.Next()
				} else {
					var received []byte
					received, err = io.ReadAll(line.Large)
					require.NotEmpty(t, received, "payload bytes must be read before the real terminal error")
				}
				require.ErrorIs(t, err, cause.err, "oversized EOF normalization must retain every independent failure")
				require.True(t, source.returned)
			})
		}
	}
}

// TestLineReaderOversizedPureEOFControls preserves clean wrapped/duplicate EOF payload completion and discard.
func TestLineReaderOversizedPureEOFControls(t *testing.T) {
	for _, terminal := range []struct {
		name string
		err  error
	}{
		{"wrapped_eof", errors.Wrap(io.EOF, "synthetic wrapper")},
		{"duplicate_eof", errors.Join(io.EOF, io.EOF)},
	} {
		for _, discard := range []bool{false, true} {
			mode := "read_payload"
			if discard {
				mode = "discard_via_next"
			}
			t.Run(terminal.name+"/"+mode, func(t *testing.T) {
				payload := strings.Repeat("x", 513)
				reader := NewLineReader(&joinedEOFTerminalReader{prefix: strings.NewReader("data: " + payload), terminal: terminal.err}, 32)
				line, err := reader.Next()
				require.NoError(t, err)
				require.True(t, line.Oversized)
				if !discard {
					received, readErr := io.ReadAll(line.Large)
					require.NoError(t, readErr)
					require.Equal(t, payload, string(received))
				}
				_, err = reader.Next()
				require.ErrorIs(t, err, io.EOF)
			})
		}
	}
}
