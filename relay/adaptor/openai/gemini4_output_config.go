package openai

import (
	"bytes"
	"encoding/json"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/one-api/relay/model"
)

// gemini4OutputControls decodes only representable controls and retains exact schema numbers.
type gemini4OutputControls struct {
	Effort *string         `json:"effort"`
	Format json.RawMessage `json:"format"`
}

// decodeGemini4OutputControls returns validated effort and a lossless Chat format.
// Unsupported controls and competing formats fail rather than weakening constraints.
func decodeGemini4OutputControls(raw json.RawMessage, existing *model.ResponseFormat) (*string, *model.ResponseFormat, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil, nil
	}
	var controls gemini4OutputControls
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&controls); err != nil {
		return nil, nil, errors.Wrap(err, "validation failed: decode Gemini output_config")
	}
	if controls.Format == nil {
		return controls.Effort, nil, nil
	}
	var format struct {
		Type        string         `json:"type"`
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Schema      map[string]any `json:"schema"`
		Strict      *bool          `json:"strict"`
	}
	decoder = json.NewDecoder(bytes.NewReader(controls.Format))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&format); err != nil {
		return nil, nil, errors.Wrap(err, "validation failed: decode output_config.format")
	}
	if format.Type != "json_schema" || format.Schema == nil {
		return nil, nil, errors.New("validation failed: output_config.format requires json_schema and an object schema")
	}
	if format.Name == "" {
		format.Name = "response"
	}
	if format.Strict == nil {
		strict := true
		format.Strict = &strict
	}
	converted := &model.ResponseFormat{Type: "json_schema", JsonSchema: &model.JSONSchema{
		Name: format.Name, Description: format.Description, Schema: format.Schema, Strict: format.Strict,
	}}
	if existing != nil {
		a, err := json.Marshal(existing)
		if err != nil {
			return nil, nil, errors.Wrap(err, "validation failed: encode response_format")
		}
		b, err := json.Marshal(converted)
		if err != nil {
			return nil, nil, errors.Wrap(err, "validation failed: encode output_config.format")
		}
		if !bytes.Equal(a, b) {
			return nil, nil, errors.New("validation failed: conflicting response_format and output_config.format")
		}
	}
	return controls.Effort, converted, nil
}
