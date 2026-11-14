package openai_compatible

import (
	"encoding/json"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/one-api/relay/adaptor/geminiOpenaiCompatible"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// hasExplicitGemini4OutputFormat reports whether the effective compatible model
// has a native format that must take precedence over implicit tool promotion.
// It reads caller data without mutation and leaves format semantics to validation.
func hasExplicitGemini4OutputFormat(info *meta.Meta, request *model.ClaudeRequest) (bool, error) {
	if !isGemini4ClaudeDestination(info, request) || len(request.OutputConfig) == 0 {
		return false, nil
	}
	var controls map[string]json.RawMessage
	if err := json.Unmarshal(request.OutputConfig, &controls); err != nil {
		return false, errors.Wrap(err, "validation failed: decode Gemini output_config before tool promotion")
	}
	for name := range controls {
		// Match encoding/json's case-insensitive struct-field recognition downstream.
		if strings.EqualFold(name, "format") {
			return true, nil
		}
	}
	return false, nil
}

// isGemini4ClaudeDestination resolves the effective compatible model without
// changing metadata shared by the controller and later conversion stages.
func isGemini4ClaudeDestination(info *meta.Meta, request *model.ClaudeRequest) bool {
	if info == nil || request == nil || info.ChannelType != channeltype.GeminiOpenAICompatible {
		return false
	}
	destination := *info
	destination.EnsureActualModelName(request.Model)
	return geminiOpenaiCompatible.IsGemini4Model(strings.ToLower(strings.TrimSpace(destination.ActualModelName)))
}
