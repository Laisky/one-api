package openai

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/model"
)

// imageStreamCompleted renders one final-image stream event of eventType with
// b64 image data and a usage object. Parameters: eventType is the event type,
// b64 the image payload, and usage the usage JSON. Returns: the SSE frame.
func imageStreamCompleted(eventType, b64, usage string) string {
	return "event: " + eventType + "\ndata: {\"type\":\"" + eventType + "\",\"b64_json\":\"" + b64 +
		"\",\"created_at\":1,\"size\":\"1024x1024\",\"quality\":\"high\",\"background\":\"opaque\",\"output_format\":\"png\",\"usage\":" + usage + "}\n\n"
}

// imageStreamPartial renders one partial-image stream event of eventType.
// Parameters: eventType is the event type and index the partial index.
// Returns: the SSE frame.
func imageStreamPartial(eventType string, index int) string {
	return "event: " + eventType + "\ndata: {\"type\":\"" + eventType + "\",\"b64_json\":\"AQ==\",\"partial_image_index\":" +
		string(rune('0'+index)) + "}\n\n"
}

// failingImageStreamBody yields body and then fails with a transport error
// instead of a clean end of stream.
type failingImageStreamBody struct{ body io.Reader }

// Read returns body bytes, then a reset error. Parameters: p is the buffer.
// Returns: the bytes read and io.ErrUnexpectedEOF-like failure at the end.
func (f *failingImageStreamBody) Read(p []byte) (int, error) {
	n, err := f.body.Read(p)
	if errors.Is(err, io.EOF) {
		return n, errors.New("connection reset by peer")
	}
	return n, err
}

// Close implements io.Closer. Parameters: none. Returns: nil.
func (f *failingImageStreamBody) Close() error { return nil }

// failingImageStreamWriter fails every client write after the first ok bytes.
type failingImageStreamWriter struct {
	gin.ResponseWriter
	ok int
}

// Write fails once the budget is spent. Parameters: p is the client payload.
// Returns: the bytes written and a broken-pipe error past the budget.
func (w *failingImageStreamWriter) Write(p []byte) (int, error) {
	if w.ok <= 0 {
		return 0, errors.New("write: broken pipe")
	}
	w.ok -= len(p)
	return w.ResponseWriter.Write(p)
}

// runImageStream drives ImageHandler with a 2xx event-stream response.
// Parameters: t owns the fixture, body is the provider body, and wrap optionally
// replaces the client writer. Returns: the client recorder, the handler error,
// the usage, and the stored stream receipt.
func runImageStream(t *testing.T, body io.ReadCloser, wrap func(gin.ResponseWriter) gin.ResponseWriter) (*httptest.ResponseRecorder, *model.ErrorWithStatusCode, *model.Usage, adaptor.ImageStreamReceipt) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	if wrap != nil {
		c.Writer = wrap(c.Writer)
	}
	header := make(http.Header)
	header.Set("Content-Type", "text/event-stream; charset=utf-8")
	header.Set("X-Request-Id", "req-stream")
	header.Set("Set-Cookie", "upstream-session=secret")
	errResp, usage := ImageHandler(c, &http.Response{StatusCode: http.StatusOK, Header: header, Body: body})
	raw, ok := c.Get(adaptor.ImageStreamReceiptKey)
	require.True(t, ok, "a relayed event stream must record its receipt")
	return recorder, errResp, usage, raw.(adaptor.ImageStreamReceipt)
}

// TestImageHandlerRelaysEventStreams covers streamed GPT Image responses: the
// events reach the client unchanged as they arrive, the final-image usage is
// returned for billing (summed across several final images, including an
// event larger than the line buffer), and every way the receipt can be
// incomplete is recorded instead of being billed as complete. Parameters: t
// runs the cases. Returns: none.
func TestImageHandlerRelaysEventStreams(t *testing.T) {
	const usage = `{"input_tokens":50,"output_tokens":4160,"total_tokens":4210,"input_tokens_details":{"text_tokens":50,"image_tokens":0}}`

	t.Run("generation_stream_is_forwarded_and_billed", func(t *testing.T) {
		stream := imageStreamPartial("image_generation.partial_image", 0) +
			": keep-alive comment\r\n\r\n" +
			imageStreamPartial("image_generation.partial_image", 1) +
			imageStreamCompleted("image_generation.completed", "AQID", usage)
		recorder, errResp, got, receipt := runImageStream(t, io.NopCloser(strings.NewReader(stream)), nil)
		require.Nil(t, errResp)
		require.Equal(t, http.StatusOK, recorder.Code)
		require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
		require.Equal(t, "req-stream", recorder.Header().Get("X-Request-Id"), "correlation headers are kept")
		require.Empty(t, recorder.Header().Get("Set-Cookie"), "provider cookies never reach the client")
		require.Equal(t, strings.ReplaceAll(stream, "\r\n", "\n"), recorder.Body.String(), "every line reaches the client")
		require.Equal(t, adaptor.ImageStreamReceipt{Completed: 1, Receipts: 1, Partials: 2}, receipt)
		require.NotNil(t, got)
		require.Equal(t, 50, got.PromptTokens)
		require.Equal(t, 4160, got.CompletionTokens)
		require.Equal(t, 4210, got.TotalTokens)
		require.Equal(t, 50, got.PromptTokensDetails.TextTokens)
	})

	t.Run("several_final_images_and_an_oversized_event_are_summed", func(t *testing.T) {
		large := strings.Repeat("A", 300_000) // beyond the 64 KiB line buffer
		stream := imageStreamCompleted("image_edit.completed", large, `{"input_tokens":1100,"output_tokens":1000,"total_tokens":2100,"input_tokens_details":{"text_tokens":100,"image_tokens":1000}}`) +
			imageStreamCompleted("image_edit.completed", "AQ==", `{"input_tokens":1100,"output_tokens":1000,"total_tokens":2100,"input_tokens_details":{"text_tokens":100,"image_tokens":1000}}`) +
			"data: [DONE]\n\n"
		recorder, errResp, got, receipt := runImageStream(t, io.NopCloser(strings.NewReader(stream)), nil)
		require.Nil(t, errResp)
		require.Equal(t, stream, recorder.Body.String(), "the oversized event reaches the client intact")
		require.Equal(t, adaptor.ImageStreamReceipt{Completed: 2, Receipts: 2}, receipt)
		require.Equal(t, 2200, got.PromptTokens)
		require.Equal(t, 2000, got.CompletionTokens)
		require.Equal(t, 4200, got.TotalTokens)
		require.Equal(t, 200, got.PromptTokensDetails.TextTokens)
		require.Equal(t, 2000, got.PromptTokensDetails.ImageTokens)
	})

	t.Run("multi_line_data_and_a_missing_final_blank_line", func(t *testing.T) {
		stream := "event: image_generation.completed\ndata: {\"type\":\"image_generation.completed\",\n" +
			"data: \"b64_json\":\"AQ==\",\"usage\":" + usage + "}"
		_, errResp, got, receipt := runImageStream(t, io.NopCloser(strings.NewReader(stream)), nil)
		require.Nil(t, errResp)
		require.Equal(t, adaptor.ImageStreamReceipt{Completed: 1, Receipts: 1}, receipt)
		require.Equal(t, 4160, got.CompletionTokens)
	})

	for _, tc := range []struct {
		name    string
		stream  string
		receipt adaptor.ImageStreamReceipt
		billed  bool
	}{
		{
			name:    "partial_images_only",
			stream:  imageStreamPartial("image_generation.partial_image", 0),
			receipt: adaptor.ImageStreamReceipt{Partials: 1},
		},
		{
			name:    "final_image_without_usage",
			stream:  "data: {\"type\":\"image_generation.completed\",\"b64_json\":\"AQ==\"}\n\n",
			receipt: adaptor.ImageStreamReceipt{Completed: 1, Interrupted: true},
		},
		{
			// An empty usage object prices nothing: not a receipt.
			name:    "final_image_with_empty_usage",
			stream:  "data: {\"type\":\"image_generation.completed\",\"b64_json\":\"AQ==\",\"usage\":{}}\n\n",
			receipt: adaptor.ImageStreamReceipt{Completed: 1, Interrupted: true},
			billed:  true,
		},
		{
			// Output only: its counters are kept for the ceiling, but it
			// is not a complete receipt.
			name:    "final_image_with_output_only_usage",
			stream:  "data: {\"type\":\"image_generation.completed\",\"b64_json\":\"AQ==\",\"usage\":{\"output_tokens\":1756}}\n\n",
			receipt: adaptor.ImageStreamReceipt{Completed: 1, Interrupted: true},
			billed:  true,
		},
		{
			name:    "error_event",
			stream:  imageStreamPartial("image_generation.partial_image", 0) + "event: error\ndata: {\"type\":\"error\",\"error\":{\"message\":\"server_error\"}}\n\n",
			receipt: adaptor.ImageStreamReceipt{Partials: 1, Failed: true},
		},
		{
			name:    "unreadable_event",
			stream:  imageStreamCompleted("image_generation.completed", "AQ==", usage) + "data: {\"type\":\n\n",
			receipt: adaptor.ImageStreamReceipt{Completed: 1, Receipts: 1, Interrupted: true},
			billed:  true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder, errResp, got, receipt := runImageStream(t, io.NopCloser(strings.NewReader(tc.stream)), nil)
			require.Nil(t, errResp, "a delivered stream is not a client error")
			require.Equal(t, tc.stream, recorder.Body.String(), "the provider's events still reach the client")
			require.Equal(t, tc.receipt, receipt)
			require.Equal(t, tc.billed, got != nil)
		})
	}

	t.Run("transport_failure_marks_the_receipt_interrupted", func(t *testing.T) {
		stream := imageStreamCompleted("image_generation.completed", "AQ==", usage)
		_, errResp, got, receipt := runImageStream(t, &failingImageStreamBody{body: strings.NewReader(stream)}, nil)
		require.NotNil(t, errResp)
		require.Equal(t, "read_stream_failed", errResp.Code)
		require.True(t, receipt.Interrupted)
		require.NotNil(t, got, "usage read before the failure is still returned for billing")
		require.Equal(t, 4160, got.CompletionTokens)
	})

	t.Run("client_write_failure_keeps_reading_the_receipt", func(t *testing.T) {
		stream := imageStreamPartial("image_generation.partial_image", 0) + imageStreamCompleted("image_generation.completed", "AQ==", usage)
		_, errResp, got, receipt := runImageStream(t, io.NopCloser(strings.NewReader(stream)), func(w gin.ResponseWriter) gin.ResponseWriter {
			return &failingImageStreamWriter{ResponseWriter: w, ok: 10}
		})
		require.NotNil(t, errResp)
		require.Equal(t, "write_response_body_failed", errResp.Code)
		require.Equal(t, adaptor.ImageStreamReceipt{Completed: 1, Receipts: 1, Partials: 1}, receipt)
		require.NotNil(t, got, "the receipt is read even though the client went away")
		require.Equal(t, 4160, got.CompletionTokens)
	})

	t.Run("event_beyond_the_readable_limit_is_forwarded_but_unbilled", func(t *testing.T) {
		previous := imageStreamEventLimit
		imageStreamEventLimit = 1 << 10
		t.Cleanup(func() { imageStreamEventLimit = previous })
		stream := imageStreamCompleted("image_generation.completed", strings.Repeat("B", 4<<10), usage)
		recorder, errResp, got, receipt := runImageStream(t, io.NopCloser(bytes.NewReader([]byte(stream))), nil)
		require.Nil(t, errResp)
		require.Equal(t, stream, recorder.Body.String())
		require.Equal(t, adaptor.ImageStreamReceipt{Interrupted: true}, receipt)
		require.Nil(t, got)
	})
}

// TestImageHandlerKeepsJSONResponses checks that a 2xx JSON image response is
// still decoded and forwarded byte for byte, and records no stream receipt.
// Parameters: t runs the case. Returns: none.
func TestImageHandlerKeepsJSONResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := `{"created":1,"data":[{"b64_json":"AQ=="}],"usage":{"input_tokens":5,"output_tokens":7,"total_tokens":12}}`
	header := make(http.Header)
	header.Set("Content-Type", "application/json")
	errResp, usage := ImageHandler(c, &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader(body))})
	require.Nil(t, errResp)
	require.Equal(t, body, recorder.Body.String())
	require.Equal(t, 7, usage.CompletionTokens)
	_, ok := c.Get(adaptor.ImageStreamReceiptKey)
	require.False(t, ok, "a JSON response has no stream receipt")
}
