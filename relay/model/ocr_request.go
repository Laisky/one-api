package model

// OCRRequest represents the canonical request payload for /v1/layout_parsing operations.
// The format mirrors the Zhipu layout_parsing API so that callers can send requests
// in the native OCR format through one-api.
type OCRRequest struct {
	Model                   string `json:"model"`
	File                    string `json:"file"`
	RequestID               string `json:"request_id,omitempty"`
	UserID                  string `json:"user_id,omitempty"`
	ReturnCropImages        *bool  `json:"return_crop_images,omitempty"`
	NeedLayoutVisualization *bool  `json:"need_layout_visualization,omitempty"`
	StartPageID             *int   `json:"start_page_id,omitempty"`
	EndPageID               *int   `json:"end_page_id,omitempty"`
}
