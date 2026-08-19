// Package openrouterprovider exposes this one-api instance as an upstream provider
// to OpenRouter. It implements the model-listing schema OpenRouter requires from
// providers (see https://openrouter.ai/docs/guides/get-started/for-providers).
package openrouterprovider

// ModelListResponse is the top-level shape of GET /openrouter/v1/models.
// It wraps the model catalog under a "data" key per OpenRouter's listing contract.
type ModelListResponse struct {
	Data []Model `json:"data"`
}

// Model describes a single model entry following OpenRouter's provider listing schema.
// Required fields are always emitted (even when zero/empty) so OpenRouter parsers
// see a stable shape; optional fields use omitempty.
type Model struct {
	ID                          string   `json:"id"`
	HuggingFaceID               string   `json:"hugging_face_id"`
	Name                        string   `json:"name"`
	Created                     int64    `json:"created"`
	Description                 string   `json:"description,omitempty"`
	InputModalities             []string `json:"input_modalities"`
	OutputModalities            []string `json:"output_modalities"`
	Quantization                string   `json:"quantization"`
	ContextLength               int32    `json:"context_length"`
	MaxOutputLength             int32    `json:"max_output_length"`
	Pricing                     Pricing  `json:"pricing"`
	SupportedSamplingParameters []string `json:"supported_sampling_parameters"`
	SupportedFeatures           []string `json:"supported_features"`
}

// Pricing follows OpenRouter's USD-per-unit string format. Empty/zero values for
// the required Prompt/Completion fields are emitted as "0"; optional fields use
// omitempty so providers do not advertise pricing they do not actually charge for.
type Pricing struct {
	Prompt         string `json:"prompt"`
	Completion     string `json:"completion"`
	Image          string `json:"image,omitempty"`
	Request        string `json:"request,omitempty"`
	InputCacheRead string `json:"input_cache_read,omitempty"`
}
