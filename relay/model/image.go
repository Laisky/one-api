package model

// ImageRequest is the shared, provider-neutral image generation and edit
// request decoded from client JSON or form bodies. Provider adaptors convert it
// into their own wire schema and must drop fields their upstream does not accept.
type ImageRequest struct {
	Model          string  `json:"model" form:"model"`
	Prompt         string  `json:"prompt" form:"prompt" binding:"required"`
	N              int     `json:"n,omitempty" form:"n"`
	Size           string  `json:"size,omitempty" form:"size"`
	Resolution     string  `json:"resolution,omitempty" form:"resolution"`
	Quality        string  `json:"quality,omitempty" form:"quality"`
	ResponseFormat *string `json:"response_format,omitempty" form:"response_format"`
	Style          string  `json:"style,omitempty" form:"style"`
	User           string  `json:"user,omitempty" form:"user"`
	// OutputFormat is the GPT Image encoded output format (png, jpeg, webp).
	// It is decoded so the gateway can refuse output formats it cannot bill.
	OutputFormat string `json:"output_format,omitempty" form:"output_format"`
	// Complexity is the GPT Image 2.6 rendering complexity (low, medium, high).
	Complexity string `json:"complexity,omitempty" form:"complexity"`
	// SafetyIdentifier is the client's stable, ideally hashed, end-user
	// identifier for OpenAI abuse detection; it supersedes the deprecated User.
	// It is user-identifying and must never be logged.
	SafetyIdentifier string `json:"safety_identifier,omitempty" form:"safety_identifier"`
	// IsEdit records the normalized relay operation for endpoint-specific billing.
	// It is internal metadata and is never serialized to upstream providers.
	IsEdit bool `json:"-" form:"-"`
	// -------------------------------------
	// additional fields
	// -------------------------------------
	ImagePrompt *string `json:"image_prompt,omitempty" form:"image_prompt"`
}

// // ToFormData converts the ImageRequest to form data
// func (r *ImageRequest) ToFormData() ([]byte, error) {

// }
