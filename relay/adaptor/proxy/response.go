package proxy

import (
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// proxyUsageMetadata safely obtains the input estimate and tokenizer model.
func proxyUsageMetadata(m *meta.Meta) (int, string) {
	if m == nil {
		return 0, ""
	}
	return m.PromptTokens, m.ActualModelName
}

// proxyUsageFromResponse shares the streaming observer's receipt semantics with
// callers that already have a captured response, including legacy regression tests.
func proxyUsageFromResponse(body []byte, m *meta.Meta) *model.Usage {
	promptTokens, modelName := proxyUsageMetadata(m)
	return model.ParseResponseUsage(body, promptTokens, func(text string) int {
		return openai.CountTokenText(text, modelName)
	})
}

// forwardProxyResponse observes bytes before writing them and always returns
// partial usage on interruption. The existing settlement path retains reserved
// quota for BillingEstimateReason; a client disconnect must not refund work.
// It does not drain a failed client indefinitely or buffer an entire SSE stream.
func forwardProxyResponse(c *gin.Context, resp *http.Response, m *meta.Meta) (*model.Usage, *model.ErrorWithStatusCode) {
	if resp == nil || resp.Body == nil {
		return nil, openai.ErrorWrapper(errors.New("proxy upstream response body is missing"), "proxy_response_missing", http.StatusBadGateway)
	}
	defer resp.Body.Close()

	stream := strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream")
	observer := model.NewResponseUsageAccumulator(stream)
	promptTokens, modelName := proxyUsageMetadata(m)
	finish := func(err error) (*model.Usage, *model.ErrorWithStatusCode) {
		if err != nil {
			observer.MarkIncomplete()
		}
		usage := observer.Finish(promptTokens, func(text string) int {
			return openai.CountTokenText(text, modelName)
		})
		if err != nil {
			return usage, openai.ErrorWrapper(errors.Wrap(err, "proxy response forwarding interrupted"), "proxy_response_interrupted", http.StatusBadGateway)
		}
		return usage, nil
	}
	for name, values := range resp.Header {
		c.Writer.Header()[name] = append([]string(nil), values...)
	}
	c.Writer.WriteHeader(resp.StatusCode)

	buffer := make([]byte, 32*1024)
	emptyReads := 0
	for {
		n, readErr := resp.Body.Read(buffer)
		if n > 0 {
			emptyReads = 0
			observer.Observe(buffer[:n])
			written, writeErr := c.Writer.Write(buffer[:n])
			if writeErr == nil && written != n {
				writeErr = io.ErrShortWrite
			}
			if writeErr != nil {
				return finish(writeErr)
			}
			if stream {
				c.Writer.Flush()
			}
		} else if readErr == nil {
			emptyReads++
			if emptyReads >= 100 {
				return finish(io.ErrNoProgress)
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return finish(nil)
			}
			return finish(readErr)
		}
	}
}
