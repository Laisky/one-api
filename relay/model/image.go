package model

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
	// -------------------------------------
	// additional fields
	// -------------------------------------
	ImagePrompt *string `json:"image_prompt,omitempty" form:"image_prompt"`
}

// // ToFormData converts the ImageRequest to form data
// func (r *ImageRequest) ToFormData() ([]byte, error) {

// }
