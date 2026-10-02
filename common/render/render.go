package render

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
)

// StringData streams an SSE data chunk to the client using the provided string payload.
func StringData(c *gin.Context, str string) {
	str = strings.TrimPrefix(str, "data: ")
	str = strings.TrimSuffix(str, "\r")
	c.Render(-1, common.CustomEvent{Data: "data: " + str})
	c.Writer.Flush()
}

// ObjectData serializes the object to JSON and streams it as an SSE chunk.
func ObjectData(c *gin.Context, object any) error {
	jsonData, err := json.Marshal(object)
	if err != nil {
		return errors.Wrapf(err, "error marshalling object")
	}
	StringData(c, string(jsonData))
	return nil
}

// Done signals the completion of an SSE stream to the client.
func Done(c *gin.Context) {
	StringData(c, "[DONE]")
}

// SSEEvent writes a complete SSE event with an optional event type prefix and data payload.
// This produces the standard SSE wire format used by the Responses API:
//
//	event: <type>\n
//	data: <payload>\n\n
//
// If eventType is empty, only the data line is written (same as StringData).
func SSEEvent(c *gin.Context, eventType string, data string) {
	data = strings.TrimPrefix(data, "data: ")
	data = strings.TrimSuffix(data, "\r")
	if eventType != "" {
		// Write the event type line directly; event types are single-line
		// tokens that need no escaping.
		payload := []byte("event: " + eventType + "\n")
		written, err := c.Writer.Write(payload)
		if err != nil {
			_ = c.Error(errors.Wrap(err, "write SSE event type"))
			c.Abort()
			return
		}
		if written != len(payload) {
			_ = c.Error(errors.Wrapf(io.ErrShortWrite,
				"write SSE event type: wrote %d of %d bytes", written, len(payload)))
			c.Abort()
			return
		}
	}
	c.Render(-1, common.CustomEvent{Data: "data: " + data})
	c.Writer.Flush()
}
