package openai_compatible

import (
	"context"
	"io"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"

	relayadaptor "github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/streaming"
)

// ObserveStreamChunk accounts a raw chunk before presentation rewriting.
func ObserveStreamChunk(c *gin.Context, chunk *ChatCompletionsStreamResponse, counters ...func(string, string) int) error {
	tracker := streaming.FromContext(c)
	if tracker == nil || chunk == nil {
		return nil
	}
	messages := make([]model.Message, 0, len(chunk.Choices))
	for _, choice := range chunk.Choices {
		messages = append(messages, choice.Delta)
	}
	return tracker.ObserveMessages(messages, chunk.Usage, counters...)
}

// streamFailureHandler is an optional terminal-error extension; older bridge
// implementations keep their existing compile-time contract.
type streamFailureHandler interface {
	HandleError(*gin.Context, *model.ErrorWithStatusCode) (bool, bool)
}

// FailStreamWithBridge emits a failed Responses terminal rather than a false
// successful completion. It does not settle, refund or fabricate usage.
func FailStreamWithBridge(c *gin.Context, failure *model.ErrorWithStatusCode, usage *model.Usage) bool {
	rw := StreamRewriterFromContext(c)
	if rw == nil || failure == nil {
		return false
	}
	if handler, ok := rw.(streamFailureHandler); ok {
		if usage != nil {
			rw.FinalizeUsage(usage)
		}
		handled, _ := handler.HandleError(c, failure)
		return handled
	}
	return false
}

// streamBodyOnce preserves close errors without double closing when cancellation
// races ordinary completion. Its callback never captures a pooled Gin context.
type streamBodyOnce struct {
	io.ReadCloser
	once sync.Once
	err  error
}

// Close closes the underlying provider transport exactly once.
func (b *streamBodyOnce) Close() error {
	b.once.Do(func() { b.err = b.ReadCloser.Close() })
	return b.err
}

// watchStreamBody assigns only one owner to request cancellation. For responses
// from our native net/http.Transport with the *same* cancellation channel as the
// incoming request, the transport already interrupts blocked reads. An extra
// AfterFunc/Close races it and can obscure context.Canceled as net.ErrClosed.
// Custom/unbound response bodies retain the existing interruptible cleanup.
// The boolean enables narrowly-scoped read-error handling at protocol DONE.
func watchStreamBody(c *gin.Context, resp *http.Response) (func(), bool) {
	ctx := context.Background()
	if c.Request != nil {
		ctx = c.Request.Context()
	}
	// Dispatch has verified the original request's cancellation identity,
	// even when http.Client.Timeout creates a derived response context.
	nativeBound := relayadaptor.ResponseTransportOwnsCancellation(c, resp)

	body := &streamBodyOnce{ReadCloser: resp.Body}
	resp.Body = body
	if nativeBound {
		return func() { _ = body.Close() }, true
	}

	stop := context.AfterFunc(ctx, func() { _ = body.Close() })
	return func() { stop(); _ = body.Close() }, false
}
