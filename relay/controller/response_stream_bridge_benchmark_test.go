package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	metalib "github.com/Laisky/one-api/relay/meta"
)

type benchmarkDiscardWriter struct {
	header http.Header
	bytes  int
}

func (w *benchmarkDiscardWriter) Header() http.Header {
	return w.header
}

func (w *benchmarkDiscardWriter) Write(p []byte) (int, error) {
	w.bytes += len(p)
	return len(p), nil
}

func (*benchmarkDiscardWriter) WriteHeader(int) {}
func (*benchmarkDiscardWriter) Flush()          {}

func newBridgeBenchmarkContext() (*gin.Context, *benchmarkDiscardWriter) {
	gin.SetMode(gin.TestMode)
	writer := &benchmarkDiscardWriter{header: make(http.Header)}
	c, _ := gin.CreateTestContext(writer)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set(ctxkey.RequestId, "benchmark-request")
	return c, writer
}

var benchmarkStreamBridgeSink openai_compatible.StreamRewriteHandler

// This contract test compares the complete SSE frame, not only its parsed JSON.
// It protects event ordering, prefixes, separators, escaping, and the terminal
// blank line while emitEvent's allocation strategy is optimized.
func TestChatToResponseStreamBridge_EmitEventExactBytes(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
	}{
		{name: "named event", eventType: "test.event"},
		{name: "data only", eventType: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			bridge := &chatToResponseStreamBridge{responseID: "resp_exact"}
			event := openai.ResponseAPIStreamEvent{
				Type: "overwritten",
				Text: "hello <world> & \"quoted\"\nnext",
			}

			expectedEvent := event
			expectedEvent.Type = tt.eventType
			expectedEvent.Id = bridge.responseID
			payload, err := json.Marshal(expectedEvent)
			require.NoError(t, err)

			var expected strings.Builder
			if tt.eventType != "" {
				expected.WriteString("event: ")
				expected.WriteString(tt.eventType)
				expected.WriteByte('\n')
			}
			expected.WriteString("data: ")
			expected.Write(payload)
			expected.WriteString("\n\n")

			bridge.emitEvent(c, tt.eventType, event)
			require.Equal(t, expected.String(), recorder.Body.String())
		})
	}
}

func BenchmarkChatToResponseStreamBridgeConstructTextOnly(b *testing.B) {
	c, _ := newBridgeBenchmarkContext()
	meta := &metalib.Meta{ActualModelName: "gpt-4o-test"}
	request := &openai.ResponseAPIRequest{Model: "gpt-4o"}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkStreamBridgeSink = newChatToResponseStreamBridge(c, meta, request)
	}
}

func BenchmarkChatToResponseStreamBridgeEmitEvent(b *testing.B) {
	c, writer := newBridgeBenchmarkContext()
	bridge := &chatToResponseStreamBridge{responseID: "resp_benchmark"}
	delta := strings.Repeat("response stream delta ", 32)
	event := openai.ResponseAPIStreamEvent{
		ItemId:       "msg_benchmark",
		OutputIndex:  0,
		ContentIndex: 0,
		Delta:        rawMessageFromString(delta),
	}

	b.SetBytes(int64(len(delta)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bridge.emitEvent(c, "response.output_text.delta", event)
	}
	if writer.bytes == 0 {
		b.Fatal("benchmark writer received no bytes")
	}
}
