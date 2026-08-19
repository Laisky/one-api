package model

import (
	"strings"

	"github.com/Laisky/errors/v2"
)

// VoiceCloneRequest represents the canonical payload for /v1/voice/clones
// operations. It mirrors Zhipu's native voice-clone contract
// (/api/paas/v4/voice/clone).
type VoiceCloneRequest struct {
	Model     string `json:"model,omitempty"`
	VoiceName string `json:"voice_name,omitempty"`
	Text      string `json:"text,omitempty"`
	Input     string `json:"input,omitempty"`
	FileID    string `json:"file_id,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// Normalize trims whitespace from all free-text fields and validates that the
// provider-required fields are present.
//
// Parameters: none.
// Returns: an error describing the first missing required field.
func (r *VoiceCloneRequest) Normalize() error {
	if r == nil {
		return errors.New("nil voice clone request")
	}

	r.Model = strings.TrimSpace(r.Model)
	r.VoiceName = strings.TrimSpace(r.VoiceName)
	r.Input = strings.TrimSpace(r.Input)
	r.FileID = strings.TrimSpace(r.FileID)
	r.RequestID = strings.TrimSpace(r.RequestID)
	r.Text = strings.TrimSpace(r.Text)

	if r.Model == "" {
		return errors.New("field model is required")
	}
	if r.VoiceName == "" {
		return errors.New("field voice_name is required")
	}
	if r.Input == "" {
		return errors.New("field input is required")
	}
	if r.FileID == "" {
		return errors.New("field file_id is required")
	}
	return nil
}
