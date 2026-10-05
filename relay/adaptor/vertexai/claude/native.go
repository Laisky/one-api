package vertexai

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	"github.com/Laisky/one-api/relay/model"
)

// ConvertClaudeRequest selects native passthrough without rewriting signed conversation history.
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	c.Set(ctxkey.RequestModel, request.Model)
	c.Set(ctxkey.ClaudeModel, request.Model)
	c.Set(ctxkey.ClaudeMessagesNative, true)
	c.Set(ctxkey.ClaudeDirectPassthrough, true)
	// Let the native Anthropic adaptor retain its tool/binding bookkeeping.
	return (&anthropic.Adaptor{}).ConvertClaudeRequest(c, request)
}

// PrepareRequestBody changes only fields owned by the Vertex transport in the prepared body.
// It never rereads the original inbound request or logs payloads and credentials.
func PrepareRequestBody(reader io.Reader) (io.Reader, error) {
	if reader == nil {
		return nil, errors.New("missing Vertex Claude body")
	}
	const maxBody = 32 << 20
	raw, err := io.ReadAll(io.LimitReader(reader, maxBody+1))
	if err != nil {
		return nil, errors.Wrap(err, "read Vertex Claude body")
	}
	if len(raw) > maxBody {
		return nil, errors.New("Vertex Claude body exceeds gateway 32 MiB limit")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, errors.Wrap(err, "decode Vertex Claude body")
	}
	if fields == nil {
		return nil, errors.New("Vertex Claude body must be an object")
	}
	delete(fields, "model")
	delete(fields, "stream")
	fields["anthropic_version"] = json.RawMessage(`"vertex-2023-10-16"`)
	// Raw field values retain signatures, exact integers and opaque future fields.
	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(fields); err != nil {
		return nil, errors.Wrap(err, "encode Vertex Claude body")
	}
	return bytes.NewReader(body.Bytes()), nil
}
