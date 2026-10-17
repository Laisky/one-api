package anthropic

import (
	"bytes"
	"encoding/json"

	"github.com/Laisky/errors/v2"
)

// contentWire has Content's exported fields without its JSON methods.
type contentWire Content

// contentKnownFields lists the typed content fields whose mutations override raw values.
var contentKnownFields = []string{
	"type", "text", "source", "id", "name", "input", "content", "tool_use_id",
	"thinking", "signature", "data",
}

// UnmarshalJSON retains provider-owned fields and a separate immutable typed view.
// Parameters: data is one native content block. Returns: a wrapped decode error.
func (c *Content) UnmarshalJSON(data []byte) error {
	// Structured native search results use content arrays or error objects.
	// Keep Content.Content's historical string API for ordinary callers while
	// retaining the reviewed native union in raw fields.
	var decodedFields struct {
		contentWire
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(data, &decodedFields); err != nil {
		return errors.Wrap(err, "decode Claude content")
	}
	typed := decodedFields.contentWire
	if len(decodedFields.Content) > 0 {
		if err := json.Unmarshal(decodedFields.Content, &typed.Content); err != nil {
			structured := bytes.TrimSpace(decodedFields.Content)
			knownResult := typed.Type == "tool_search_tool_result" || typed.Type == "web_search_tool_result"
			if !knownResult || len(structured) == 0 || (structured[0] != '[' && structured[0] != '{') {
				return errors.Wrap(err, "decode Claude content value")
			}
		}
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return errors.Wrap(err, "retain Claude content fields")
	}
	snapshotBytes, err := json.Marshal(typed)
	if err != nil {
		return errors.Wrap(err, "snapshot Claude content")
	}
	var snapshot map[string]json.RawMessage
	if err := json.Unmarshal(snapshotBytes, &snapshot); err != nil {
		return errors.Wrap(err, "decode Claude content snapshot")
	}
	decoded := Content(typed)
	decoded.rawFields, decoded.typedSnapshot = raw, snapshot
	*c = decoded
	return nil
}

// MarshalJSON preserves unchanged raw fields and applies every typed edit/removal.
// Parameters: c is the block to serialize. Returns: a wrapped encoding error.
// Opaque strings and numeric tokens retain their values; JSON whitespace is not an identity.
func (c Content) MarshalJSON() ([]byte, error) {
	typedBytes, err := json.Marshal(contentWire(c))
	if err != nil {
		return nil, errors.Wrap(err, "encode typed Claude content")
	}
	if len(c.rawFields) == 0 {
		return typedBytes, nil
	}
	var typed map[string]json.RawMessage
	if err := json.Unmarshal(typedBytes, &typed); err != nil {
		return nil, errors.Wrap(err, "decode typed Claude content fields")
	}
	merged := make(map[string]json.RawMessage, len(c.rawFields)+len(typed))
	for key, value := range c.rawFields {
		merged[key] = value
	}
	for _, key := range contentKnownFields {
		next, nextPresent := typed[key]
		previous, previousPresent := c.typedSnapshot[key]
		if nextPresent == previousPresent && bytes.Equal(next, previous) {
			continue
		}
		if nextPresent {
			merged[key] = next
		} else {
			delete(merged, key)
		}
	}
	result, err := json.Marshal(merged)
	return result, errors.Wrap(err, "encode lossless Claude content")
}
