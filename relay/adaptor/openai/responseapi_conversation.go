package openai

import (
	"bytes"
	"encoding/json"

	"github.com/Laisky/errors/v2"
)

// ResponseAPIConversation represents the Responses API `conversation` selector,
// which OpenAI accepts in two shapes:
//
//   - a bare conversation ID string: "conv_123"
//   - an object carrying the ID: {"id": "conv_123", ...}
//
// Both shapes are canonicalized to the same Id so downstream state resolution
// can treat them uniformly (acceptance row A02), while the original wire form is
// preserved in raw so native pass-through forwarding stays byte-faithful.
type ResponseAPIConversation struct {
	// Id is the canonical conversation identifier extracted from either selector
	// shape. It is empty only when the selector object omitted the id field.
	Id string
	// raw retains the exact bytes the client sent so re-marshaling does not alter
	// the selector shape for native upstreams that understand the object form.
	raw json.RawMessage
}

// ConversationID returns the canonical conversation identifier, or the empty
// string when no selector was supplied.
func (c *ResponseAPIConversation) ConversationID() string {
	if c == nil {
		return ""
	}
	return c.Id
}

// UnmarshalJSON accepts both the string and object selector shapes.
func (c *ResponseAPIConversation) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}

	c.raw = append(c.raw[:0], trimmed...)

	// String selector form: "conv_123".
	var asString string
	if err := json.Unmarshal(trimmed, &asString); err == nil {
		c.Id = asString
		return nil
	}

	// Object selector form: {"id": "conv_123", ...}.
	var asObject struct {
		Id string `json:"id"`
	}
	if err := json.Unmarshal(trimmed, &asObject); err != nil {
		return errors.Wrap(err, "ResponseAPIConversation.UnmarshalJSON: unsupported conversation selector shape")
	}
	c.Id = asObject.Id
	return nil
}

// MarshalJSON re-emits the original selector bytes when available so native
// forwarding preserves the client's shape; otherwise it emits the canonical
// object form built from Id.
func (c ResponseAPIConversation) MarshalJSON() ([]byte, error) {
	if len(c.raw) > 0 {
		return c.raw, nil
	}
	if c.Id == "" {
		return []byte("null"), nil
	}
	b, err := json.Marshal(struct {
		Id string `json:"id"`
	}{Id: c.Id})
	if err != nil {
		return nil, errors.Wrap(err, "ResponseAPIConversation.MarshalJSON")
	}
	return b, nil
}
