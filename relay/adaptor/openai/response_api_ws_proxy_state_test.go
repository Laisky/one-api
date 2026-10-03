package openai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestResponseAPIWSStoreCollector verifies the websocket collector captures only
// store!=false completed responses, dedupes by id, and ignores non-terminal
// frames (proposal ST-011 / Section 5.9, SEC06).
func TestResponseAPIWSStoreCollector(t *testing.T) {
	col := &responseAPIWSStoreCollector{}

	// store defaults to true -> collected.
	col.collect([]byte(`{"type":"response.completed","response":{"id":"resp_up1","model":"gpt-5","output":[],"usage":{"input_tokens":1}}}`))
	require.Len(t, col.responses, 1)

	// duplicate id -> not collected again.
	col.collect([]byte(`{"type":"response.completed","response":{"id":"resp_up1"}}`))
	require.Len(t, col.responses, 1)

	// explicit store=false -> connection-local, excluded.
	col.collect([]byte(`{"type":"response.completed","response":{"id":"resp_up2","store":false}}`))
	require.Len(t, col.responses, 1)

	// non-terminal frames are ignored.
	col.collect([]byte(`{"type":"response.created","response":{"id":"resp_up3"}}`))
	col.collect([]byte(`{"type":"response.output_text.delta","delta":"x"}`))
	require.Len(t, col.responses, 1)

	// explicit store=true -> collected.
	col.collect([]byte(`{"type":"response.completed","response":{"id":"resp_up4","store":true}}`))
	require.Len(t, col.responses, 2)
	require.Equal(t, "resp_up1", col.responses[0].Id)
	require.Equal(t, "resp_up4", col.responses[1].Id)

	// malformed / empty -> no panic, no change.
	col.collect(nil)
	col.collect([]byte(`not json`))
	require.Len(t, col.responses, 2)
}
