package openai

import (
	"github.com/Laisky/errors/v2"
	"github.com/Laisky/one-api/relay/model"
)

// UnmarshalJSON retains exact numeric bounds in Responses text.format schemas
// without changing number decoding in request inputs, arguments or metadata.
func (f *ResponseTextFormat) UnmarshalJSON(data []byte) error {
	type wireFormat ResponseTextFormat
	var decoded wireFormat
	if err := model.DecodeSchemaJSON(data, &decoded); err != nil {
		return errors.Wrap(err, "decode response text format")
	}
	*f = ResponseTextFormat(decoded)
	return nil
}
