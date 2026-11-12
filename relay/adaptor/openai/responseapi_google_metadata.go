package openai

import (
	"encoding/json"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/model"
)

// restoreGoogleCallMetadata restores one Google-native call's gateway metadata.
// Parameters: tool is the lowered call and value is decoded client metadata.
// Returns a wrapped validation error for malformed metadata, preserving opaque signatures.
// Callers select this only for native Google providers; other provider contracts are unchanged.
func restoreGoogleCallMetadata(tool *model.Tool, value any) error {
	if value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return errors.Wrap(err, "encode Google call metadata")
	}
	var extra model.ToolExtraContent
	if err := json.Unmarshal(raw, &extra); err != nil {
		return errors.Wrap(err, "decode Google call metadata")
	}
	_, positioned := model.GeminiPartIndex(&extra)
	textPresence := extra.OneAPI != nil && extra.OneAPI.GeminiTextPresent != nil
	if signature, thought := model.GoogleMetadata(&extra); signature != "" || thought || positioned || textPresence {
		tool.ExtraContent = &extra
	}
	return nil
}
