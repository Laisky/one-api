package sse

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLineReader_RecognizesLineKinds verifies the reader preserves SSE line types.
func TestLineReader_RecognizesLineKinds(t *testing.T) {
	t.Parallel()

	input := strings.Join([]string{
		": keepalive",
		"",
		"event: response.created",
		"data: {\"ok\":true}",
		"other",
		"tail without newline",
	}, "\r\n")

	reader := NewLineReader(strings.NewReader(input), 64)
	expected := []struct {
		kind   LineKind
		prefix string
		text   string
	}{
		{kind: LineKindComment, prefix: commentPrefix, text: ": keepalive"},
		{kind: LineKindBlank, prefix: "", text: ""},
		{kind: LineKindEvent, prefix: eventLinePrefix, text: "event: response.created"},
		{kind: LineKindData, prefix: dataLinePrefix, text: "data: {\"ok\":true}"},
		{kind: LineKindOther, prefix: "", text: "other"},
		{kind: LineKindOther, prefix: "", text: "tail without newline"},
	}

	for _, want := range expected {
		line, err := reader.Next()
		require.NoError(t, err)
		require.False(t, line.Oversized)
		require.Equal(t, want.kind, line.Kind)
		require.Equal(t, want.prefix, line.Prefix)
		require.Equal(t, want.text, line.Text())
	}

	_, err := reader.Next()
	require.ErrorIs(t, err, io.EOF)
}

// TestLineReader_OversizedDataLineStreamsPayload verifies oversized data lines stream correctly.
func TestLineReader_OversizedDataLineStreamsPayload(t *testing.T) {
	t.Parallel()

	payload := strings.Repeat("x", 4096)
	reader := NewLineReader(strings.NewReader("data: "+payload+"\r\n"), 32)

	line, err := reader.Next()
	require.NoError(t, err)
	require.True(t, line.Oversized)
	require.Equal(t, LineKindData, line.Kind)
	require.Equal(t, dataLinePrefix, line.Prefix)
	require.Nil(t, line.Small)
	require.NotNil(t, line.Large)

	data, err := io.ReadAll(line.Large)
	require.NoError(t, err)
	require.Equal(t, payload, string(data))

	_, err = reader.Next()
	require.ErrorIs(t, err, io.EOF)
}

// TestLineReader_NextDiscardsUnreadOversizedLine verifies Next can advance after a partial read.
func TestLineReader_NextDiscardsUnreadOversizedLine(t *testing.T) {
	t.Parallel()

	payload := strings.Repeat("y", 2048)
	reader := NewLineReader(strings.NewReader("data: "+payload+"\ndata: second\n"), 32)

	line, err := reader.Next()
	require.NoError(t, err)
	require.True(t, line.Oversized)

	buf := make([]byte, 16)
	_, err = line.Large.Read(buf)
	require.NoError(t, err)

	nextLine, err := reader.Next()
	require.NoError(t, err)
	require.False(t, nextLine.Oversized)
	require.Equal(t, "data: second", nextLine.Text())
}

// TestLineReader_EOFWithoutTrailingNewline verifies the final line is returned before EOF.
func TestLineReader_EOFWithoutTrailingNewline(t *testing.T) {
	t.Parallel()

	reader := NewLineReader(strings.NewReader("data: tail"), 64)

	line, err := reader.Next()
	require.NoError(t, err)
	require.False(t, line.Oversized)
	require.Equal(t, LineKindData, line.Kind)
	require.Equal(t, "data: tail", line.Text())

	_, err = reader.Next()
	require.ErrorIs(t, err, io.EOF)
}

// TestLineReader_OversizedNonDataLineReturnsErrLineTooLong verifies oversized non-data lines fail fast.
func TestLineReader_OversizedNonDataLineReturnsErrLineTooLong(t *testing.T) {
	t.Parallel()

	reader := NewLineReader(strings.NewReader("event: "+strings.Repeat("z", 128)+"\n"), 16)

	_, err := reader.Next()
	require.Error(t, err)
	require.ErrorIs(t, err, ErrLineTooLong)
}
