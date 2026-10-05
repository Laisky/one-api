package openai

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
)

// TestWebSearchStreamCounter verifies the pass-through counter splits lines
// across reads, deduplicates repeated items, ignores non-search actions and
// skips oversized lines without altering the forwarded bytes.
func TestWebSearchStreamCounter(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	stream := strings.Join([]string{
		`data: {"type":"response.output_item.done","item":{"type":"web_search_call","id":"ws_1","action":{"type":"search","query":"a"}}}`,
		`data: {"type":"response.output_item.done","item":{"type":"web_search_call","id":"ws_2","action":{"type":"open_page"}}}`,
		`data: {"type":"response.output_item.done","item":{"type":"web_search_call","id":"ws_3","action":{"type":"search","query":"b"}}}`,
		`data: {"type":"response.output_text.delta","delta":"web_search_call is only text here"}`,
		`data: ` + strings.Repeat("x", maxWebSearchCounterLine+10) + `web_search_call`,
		`data: {"type":"response.completed","response":{"object":"response","output":[{"type":"web_search_call","id":"ws_1","action":{"type":"search"}},{"type":"web_search_call","id":"ws_3","action":{"type":"search"}}]}}`,
		`data: [DONE]`,
		``,
	}, "\n")
	counter := newWebSearchStreamCounter(io.NopCloser(iotest.OneByteReader(strings.NewReader(stream[:2048]))))
	forwarded, err := io.ReadAll(counter)
	require.NoError(t, err)
	require.Equal(t, stream[:2048], string(forwarded))

	counter = newWebSearchStreamCounter(io.NopCloser(iotest.HalfReader(strings.NewReader(stream))))
	forwarded, err = io.ReadAll(counter)
	require.NoError(t, err)
	require.Equal(t, stream, string(forwarded), "counting never changes the converted bytes")
	require.Equal(t, 2, counter.count, "two distinct chargeable searches")
	require.NoError(t, counter.Close())

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	counter.record(c)
	require.Equal(t, 2, c.GetInt(ctxkey.WebSearchCallCount))
}
