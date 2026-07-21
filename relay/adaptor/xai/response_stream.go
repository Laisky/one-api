package xai

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// responseAPIStreamEvent is the minimal shape needed to recover billing data from
// an x.AI Response API SSE stream. Usage is published on the terminal
// response.completed event inside the nested response object; a top-level usage
// field is accepted as well so a provider-side schema change degrades to working
// billing rather than to silence.
type responseAPIStreamEvent struct {
	Type     string            `json:"type"`
	Usage    *ResponseAPIUsage `json:"usage"`
	Response *struct {
		Usage *ResponseAPIUsage `json:"usage"`
	} `json:"response"`
}

// streamResponseAPI forwards an x.AI Response API SSE stream to the client
// verbatim while extracting the usage block that billing needs.
//
// Byte fidelity is deliberate: every line read is written back unchanged, so the
// client sees exactly what x.AI sent, including event ordering, unknown event
// types and any provider-specific fields. The only added behaviour is a flush per
// line — without it the stream would sit in the server's write buffer — and the
// usage sniffing itself, which never alters the forwarded bytes.
//
// Returning a nil usage is still possible (a truncated stream, or a provider that
// stops emitting usage). That is reported to the caller as-is; the billing layer
// is responsible for settling such a request at its pre-consumed estimate rather
// than dropping it, so a missing usage can never produce an invisible charge.
func (a *Adaptor) streamResponseAPI(c *gin.Context, resp *http.Response, _ *meta.Meta) (*model.Usage, *model.ErrorWithStatusCode) {
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, value := range values {
			c.Writer.Header().Add(key, value)
		}
	}
	if resp.Header.Get("Content-Type") == "" {
		c.Writer.Header().Set("Content-Type", "text/event-stream")
	}
	c.Writer.WriteHeader(resp.StatusCode)

	flusher, canFlush := c.Writer.(http.Flusher)
	reader := bufio.NewReader(resp.Body)

	var usage *ResponseAPIUsage
	for {
		// ReadBytes has no token-size ceiling, so an oversized event (a large
		// base64 payload, for instance) is forwarded rather than truncated.
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			if _, writeErr := c.Writer.Write(line); writeErr != nil {
				return usage.toModelUsage(), openai_compatible.ErrorWrapper(
					errors.Wrap(writeErr, "write stream chunk to client"),
					"write_response_body_failed", http.StatusInternalServerError)
			}
			if canFlush {
				flusher.Flush()
			}
			if found := extractStreamUsage(line); found != nil {
				usage = found
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			// The client already holds a partial stream, so the request is not a
			// failure it can retry; report the usage recovered so far and let the
			// caller bill for what was actually delivered.
			return usage.toModelUsage(), openai_compatible.ErrorWrapper(
				errors.Wrap(readErr, "read upstream stream"),
				"read_response_body_failed", http.StatusInternalServerError)
		}
	}

	return usage.toModelUsage(), nil
}

// extractStreamUsage returns the usage block carried by one SSE line, or nil when
// the line carries none. Malformed payloads are ignored: a stream must never fail
// because one event could not be parsed.
func extractStreamUsage(line []byte) *ResponseAPIUsage {
	trimmed := strings.TrimSpace(string(line))
	if !strings.HasPrefix(trimmed, "data:") {
		return nil
	}
	payload := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
	if payload == "" || payload == "[DONE]" {
		return nil
	}

	var event responseAPIStreamEvent
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return nil
	}
	if event.Response != nil && event.Response.Usage != nil {
		return event.Response.Usage
	}
	return event.Usage
}

// toModelUsage converts a recovered usage block, tolerating a nil receiver so
// callers can hand back "nothing was reported" without a nil check at each site.
func (u *ResponseAPIUsage) toModelUsage() *model.Usage {
	if u == nil {
		return nil
	}
	return u.ToModelUsage()
}
