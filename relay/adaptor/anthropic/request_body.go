package anthropic

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
)

// PrepareRequestBody applies known-model compatibility at the final prepared HTTP boundary.
// Unknown models keep their original reader, and no retry or second inference is introduced.
func PrepareRequestBody(c *gin.Context, name string, reader io.Reader) (io.Reader, error) {
	name = CompatibilityModel(c, name)
	if !IsClaudeSonnet55(name) && !IsClaudeHaiku55(name) {
		return reader, nil
	}
	if reader == nil {
		return nil, errors.New("validation failed: missing Claude body")
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxClaudeHTTPPayload+1))
	if err != nil {
		return nil, errors.Wrap(err, "read prepared Claude body")
	}
	if len(raw) > maxClaudeHTTPPayload {
		return nil, errors.New("validation failed: Claude body exceeds gateway 32 MiB limit")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, errors.Wrap(err, "validation failed: invalid Claude JSON")
	}
	if err := NormalizeSonnet55Controls(name, fields); err != nil {
		return nil, err
	}
	if err := NormalizeHaiku55Controls(name, fields); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(fields); err != nil {
		return nil, errors.Wrap(err, "encode prepared Claude body")
	}
	return bytes.NewReader(out.Bytes()), nil
}
