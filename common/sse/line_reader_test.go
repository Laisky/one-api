package sse

import (
	"encoding/json"
	"fmt"
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

// TestLineReader_OversizedJSONDecode verifies that json.NewDecoder can parse
// a JSON object streamed through line.Large when the payload far exceeds the buffer.
func TestLineReader_OversizedJSONDecode(t *testing.T) {
	t.Parallel()

	const bufSize = 64 // tiny buffer

	type streamResponse struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Content string `json:"content"`
	}

	// Generate a payload ~512 KB, well beyond the 64-byte buffer.
	largeContent := strings.Repeat("Hello world! This is a streaming chunk. ", 13000) // ~520 KB
	want := streamResponse{
		ID:      "chatcmpl-oversized",
		Object:  "chat.completion.chunk",
		Content: largeContent,
	}

	jsonBytes, err := json.Marshal(want)
	require.NoError(t, err)
	require.Greater(t, len(jsonBytes), bufSize*100,
		"payload must be far larger than buffer to exercise streaming")

	input := "data: " + string(jsonBytes) + "\n"
	reader := NewLineReader(strings.NewReader(input), bufSize)

	line, err := reader.Next()
	require.NoError(t, err)
	require.True(t, line.Oversized)
	require.NotNil(t, line.Large)

	var got streamResponse
	require.NoError(t, json.NewDecoder(line.Large).Decode(&got))
	require.Equal(t, want, got)
}

// TestLineReader_MultipleOversizedJSONLines verifies sequential oversized
// JSON lines are each decoded correctly, with intermediate normal lines.
func TestLineReader_MultipleOversizedJSONLines(t *testing.T) {
	t.Parallel()

	const bufSize = 128

	type chunk struct {
		Index   int    `json:"index"`
		Payload string `json:"payload"`
	}

	var sb strings.Builder
	chunks := make([]chunk, 5)
	for i := range chunks {
		// Each payload is ~200 KB, well beyond the 128-byte buffer.
		chunks[i] = chunk{
			Index:   i,
			Payload: strings.Repeat(fmt.Sprintf("chunk-%d-data-", i), 15000),
		}
		jsonBytes, err := json.Marshal(chunks[i])
		require.NoError(t, err)
		require.Greater(t, len(jsonBytes), bufSize*100)

		sb.WriteString("data: " + string(jsonBytes) + "\n")
		// Insert a normal-sized event line between data lines.
		if i < len(chunks)-1 {
			sb.WriteString("event: delta\n")
		}
	}

	reader := NewLineReader(strings.NewReader(sb.String()), bufSize)
	for i, want := range chunks {
		line, err := reader.Next()
		require.NoError(t, err, "chunk %d", i)
		require.True(t, line.Oversized, "chunk %d should be oversized", i)

		var got chunk
		require.NoError(t, json.NewDecoder(line.Large).Decode(&got), "chunk %d decode", i)
		require.Equal(t, want, got, "chunk %d mismatch", i)

		// Read the intermediate event line (except after the last chunk).
		if i < len(chunks)-1 {
			evLine, err := reader.Next()
			require.NoError(t, err)
			require.False(t, evLine.Oversized)
			require.Equal(t, LineKindEvent, evLine.Kind)
		}
	}

	_, err := reader.Next()
	require.ErrorIs(t, err, io.EOF)
}

// TestLineReader_OversizedJSONDecodeWithSkippedLines verifies that calling
// Next() without reading line.Large correctly discards the oversized payload
// and the following lines are still parseable.
func TestLineReader_OversizedJSONDecodeWithSkippedLines(t *testing.T) {
	t.Parallel()

	const bufSize = 64

	// First line: oversized data we will skip (not read line.Large).
	bigPayload := `{"skipped":"` + strings.Repeat("x", 300000) + `"}`
	// Second line: oversized data we will actually decode.
	wantContent := strings.Repeat("important-data-", 20000)
	smallJSON := fmt.Sprintf(`{"content":"%s"}`, wantContent)

	input := "data: " + bigPayload + "\n" +
		"data: " + smallJSON + "\n"

	reader := NewLineReader(strings.NewReader(input), bufSize)

	// Read first line but do NOT consume line.Large.
	line1, err := reader.Next()
	require.NoError(t, err)
	require.True(t, line1.Oversized)

	// Immediately read next line — the reader should discard the skipped payload.
	line2, err := reader.Next()
	require.NoError(t, err)
	require.True(t, line2.Oversized)
	require.NotNil(t, line2.Large)

	var got struct {
		Content string `json:"content"`
	}
	require.NoError(t, json.NewDecoder(line2.Large).Decode(&got))
	require.Equal(t, wantContent, got.Content)
}

// TestLineReader_OversizedJSONWithNestedObjects verifies streaming decode of
// deeply nested JSON structures that exceed the buffer by a large margin.
func TestLineReader_OversizedJSONWithNestedObjects(t *testing.T) {
	t.Parallel()

	const bufSize = 256

	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type choice struct {
		Index   int     `json:"index"`
		Delta   message `json:"delta"`
		Logprob *string `json:"logprob"`
	}
	type streamResp struct {
		ID      string   `json:"id"`
		Object  string   `json:"object"`
		Model   string   `json:"model"`
		Choices []choice `json:"choices"`
	}

	// Build a response with multiple choices, each with large content (~100 KB each).
	want := streamResp{
		ID:     "chatcmpl-mega",
		Object: "chat.completion.chunk",
		Model:  "gpt-4o-test",
	}
	for i := 0; i < 3; i++ {
		want.Choices = append(want.Choices, choice{
			Index: i,
			Delta: message{
				Role:    "assistant",
				Content: strings.Repeat(fmt.Sprintf("choice%d-token-", i), 8000),
			},
		})
	}

	jsonBytes, err := json.Marshal(want)
	require.NoError(t, err)
	require.Greater(t, len(jsonBytes), bufSize*100,
		"nested JSON must far exceed buffer size")

	reader := NewLineReader(strings.NewReader("data: "+string(jsonBytes)+"\n"), bufSize)

	line, err := reader.Next()
	require.NoError(t, err)
	require.True(t, line.Oversized)

	var got streamResp
	require.NoError(t, json.NewDecoder(line.Large).Decode(&got))
	require.Equal(t, want.ID, got.ID)
	require.Equal(t, want.Object, got.Object)
	require.Equal(t, want.Model, got.Model)
	require.Len(t, got.Choices, 3)
	for i, c := range got.Choices {
		require.Equal(t, want.Choices[i].Index, c.Index)
		require.Equal(t, want.Choices[i].Delta, c.Delta)
	}
}
