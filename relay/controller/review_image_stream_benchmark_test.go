package controller

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// BenchmarkResponseBridgeImageOutput measures bounded data-URL output allocations without changing the wire format.
func BenchmarkResponseBridgeImageOutput(b *testing.B) {
	for _, size := range []int{64 << 10, 1 << 20} {
		b.Run(fmt.Sprintf("payload_%d", size), func(b *testing.B) {
			url := "data:image/png;base64," + strings.Repeat("A", size)
			chunk := &openai_compatible.ChatCompletionsStreamResponse{Choices: []openai_compatible.ChatCompletionsStreamResponseChoice{{Delta: model.Message{Content: []model.MessageContent{{Type: model.ContentTypeImageURL, ImageURL: &model.ImageURL{Url: url}}}}}}}
			b.SetBytes(int64(len(url)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				c, w := newBridgeBenchmarkContext()
				bridge := newChatToResponseStreamBridge(c, &metalib.Meta{ActualModelName: "benchmark"}, &openai.ResponseAPIRequest{Model: "benchmark"}).(*chatToResponseStreamBridge)
				bridge.HandleChunk(c, chunk)
				bridge.HandleDone(c)
				if bridge.StreamWriteError() != nil || w.bytes == 0 {
					b.Fatal("image stream was not written")
				}
			}
		})
	}
}

type reviewImageDisconnectWriter struct{ gin.ResponseWriter }

func (w reviewImageDisconnectWriter) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "data:image/png;base64,") {
		return 0, errors.New("client disconnected")
	}
	return w.ResponseWriter.Write(p)
}

// TestLargeImageDisconnectStopsEmission checks a bounded image frame is not counted or emitted after a failed write.
func TestLargeImageDisconnectStopsEmission(t *testing.T) {
	c, rec := newBridgeTestContext(t)
	c.Writer = reviewImageDisconnectWriter{c.Writer}
	bridge := newTestBridge(t, c)
	url := "data:image/png;base64," + strings.Repeat("A", 1<<20)
	chunk := &openai_compatible.ChatCompletionsStreamResponse{Choices: []openai_compatible.ChatCompletionsStreamResponseChoice{{Delta: model.Message{Content: []model.MessageContent{{Type: model.ContentTypeImageURL, ImageURL: &model.ImageURL{Url: url}}}}}}}
	bridge.HandleChunk(c, chunk)
	require.Error(t, bridge.StreamWriteError())
	require.Zero(t, bridge.DeliveredImagesInLastChunk())
	before := rec.Body.Len()
	bridge.HandleChunk(c, bridgeTextChunk("late"))
	bridge.HandleDone(c)
	require.Equal(t, before, rec.Body.Len())
	require.NotContains(t, rec.Body.String(), "response.completed")
}
