package openai

import (
	"github.com/Laisky/one-api/relay/model"
)

// responseGoogleMetadata decodes a typed Google carrier using the shared opaque tool-signature contract.
func responseGoogleMetadata(value any) (*model.ToolExtraContent, error) {
	var carrier model.Tool
	if err := restoreGoogleCallMetadata(&carrier, value); err != nil {
		return nil, err
	}
	return carrier.ExtraContent, nil
}

// responseGoogleMessageParts preserves message-level metadata when one native Google assistant turn joins text and calls.
func responseGoogleMessageParts(message *model.Message) []model.MessageContent {
	if message.IsStringContent() {
		text := message.StringContent()
		if text == "" && message.ExtraContent == nil {
			return nil
		}
		return []model.MessageContent{{Type: model.ContentTypeText, Text: &text, ExtraContent: message.ExtraContent}}
	}
	// Native Responses lowering constructs typed parts before this merge.
	if parts, ok := message.Content.([]model.MessageContent); ok {
		return append([]model.MessageContent(nil), parts...)
	}
	return message.ParseContent()
}

// responseHasGoogleTextMetadata reports whether an assistant message must retain positional content instead of string aggregation.
func responseHasGoogleTextMetadata(message *model.Message) bool {
	if message.ExtraContent != nil {
		return true
	}
	if parts, ok := message.Content.([]model.MessageContent); ok {
		for _, part := range parts {
			if part.ExtraContent != nil {
				return true
			}
		}
	}
	return false
}
