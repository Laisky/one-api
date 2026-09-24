package sse

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// bufferedReadProbe counts upstream calls to prove buffered operations never perform I/O.
type bufferedReadProbe struct {
	reader *strings.Reader
	reads  int
}

// Read records one upstream read and copies bytes from the fixture into p.
func (p *bufferedReadProbe) Read(data []byte) (int, error) {
	p.reads++
	return p.reader.Read(data)
}

// TestNextBufferedNeverReadsUpstream covers empty, complete, partial, CRLF and retained-slice ownership behavior.
func TestNextBufferedNeverReadsUpstream(t *testing.T) {
	t.Parallel()
	probe := &bufferedReadProbe{reader: strings.NewReader("data: first\r\n\r\ndata: 世界🙂\r\npartial")}
	reader := NewLineReader(probe, 256)
	_, ready, err := reader.NextBuffered()
	require.NoError(t, err)
	require.False(t, ready)
	require.Zero(t, probe.reads)
	first, err := reader.Next()
	require.NoError(t, err)
	reads := probe.reads
	blank, ready, err := reader.NextBuffered()
	require.NoError(t, err)
	require.True(t, ready)
	require.Equal(t, LineKindBlank, blank.Kind)
	line, ready, err := reader.NextBuffered()
	require.NoError(t, err)
	require.True(t, ready)
	require.Equal(t, "data: 世界🙂", line.Text())
	_, ready, err = reader.NextBuffered()
	require.NoError(t, err)
	require.False(t, ready)
	require.Equal(t, reads, probe.reads)
	tail, err := reader.Next()
	require.NoError(t, err)
	require.Equal(t, "partial", tail.Text())
	require.Equal(t, "data: first", first.Text())
	require.Equal(t, "data: 世界🙂", line.Text())
}

// TestNextBufferedPreservesOversizedOwnership ensures a caller-owned large payload is neither drained nor mutated.
func TestNextBufferedPreservesOversizedOwnership(t *testing.T) {
	t.Parallel()
	payload := strings.Repeat("世界🙂", 12000)
	probe := &bufferedReadProbe{reader: strings.NewReader("data: " + payload + "\n\ndata: [DONE]\n\n")}
	reader := NewLineReader(probe, 256)
	large, err := reader.Next()
	require.NoError(t, err)
	require.True(t, large.Oversized)
	reads := probe.reads
	_, ready, err := reader.NextBuffered()
	require.NoError(t, err)
	require.False(t, ready)
	require.Equal(t, reads, probe.reads)
	actual, err := io.ReadAll(large.Large)
	require.NoError(t, err)
	require.Equal(t, payload, string(actual))
	blank, err := reader.Next()
	require.NoError(t, err)
	require.Equal(t, LineKindBlank, blank.Kind)
	terminal, err := reader.Next()
	require.NoError(t, err)
	require.Equal(t, "data: [DONE]", terminal.Text())
}
