package model

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/Laisky/errors/v2"
)

// UnmarshalJSON preserves optional native objects without changing the text-only
// fields consumed by existing validators, accounting, and adaptors. The controller
// must check HasStructuredInput before allowing an adaptor to send the request.
func (r *RerankRequest) UnmarshalJSON(data []byte) error {
	type plain RerankRequest
	var decoded plain
	wire := struct {
		*plain
		Query     json.RawMessage   `json:"query"`
		Documents []json.RawMessage `json:"documents"`
	}{plain: &decoded}
	if err := json.Unmarshal(data, &wire); err != nil {
		return errors.Wrap(err, "decode rerank request")
	}
	if len(wire.Query) > 0 && !bytes.Equal(bytes.TrimSpace(wire.Query), []byte("null")) {
		text, structured, image, err := rerankInputText(wire.Query)
		if err != nil {
			return errors.Wrap(err, "decode rerank query")
		}
		if structured && !image {
			return errors.New("rerank query must be a string or an image object")
		}
		decoded.Query = text
		if structured {
			decoded.NativeQuery = append(json.RawMessage(nil), wire.Query...)
		}
	}
	if wire.Documents != nil {
		decoded.Documents = make([]string, len(wire.Documents))
	}
	structuredDocuments := false
	for i, raw := range wire.Documents {
		text, structured, _, err := rerankInputText(raw)
		if err != nil {
			return errors.Wrapf(err, "decode rerank document %d", i)
		}
		decoded.Documents[i] = text
		structuredDocuments = structuredDocuments || structured
	}
	if structuredDocuments {
		decoded.NativeDocuments = wire.Documents
	}
	*r = RerankRequest(decoded)
	return nil
}

// rerankInputText validates one text/image item and returns its text estimate,
// whether it is structured, and whether it is an image. Image tokens are settled
// from upstream usage; counting a URL or base64 representation would be incorrect.
func rerankInputText(raw json.RawMessage) (string, bool, bool, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", false, false, errors.New("rerank input cannot be null")
	}
	if raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return "", false, false, errors.Wrap(err, "decode rerank text")
		}
		return text, false, false, nil
	}
	var item map[string]string
	if err := json.Unmarshal(raw, &item); err != nil {
		return "", false, false, errors.Wrap(err, "rerank input must be text or a text/image object")
	}
	if len(item) != 1 {
		return "", false, false, errors.New("rerank object must contain exactly one text or image field")
	}
	if text, ok := item["text"]; ok {
		return text, true, false, nil
	}
	if image, ok := item["image"]; ok && strings.TrimSpace(image) != "" {
		return "[image]", true, true, nil
	}
	return "", false, false, errors.New("rerank object requires text or a nonempty image")
}
