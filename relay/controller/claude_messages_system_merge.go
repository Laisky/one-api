package controller

import (
	"strings"

	relayadaptor "github.com/Laisky/one-api/relay/adaptor"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// systemMergeSeparator joins merged mid-array system text with its carrier turn's
// existing content.
const systemMergeSeparator = "\n\n"

// mergeMidArraySystemMessages removes role:"system" messages from the messages
// array and folds their text into an adjacent turn. It prefers the following
// turn, falls back to the preceding turn for trailing system entries, and never
// edits the top-level System field. Assistant turns with signed thinking blocks
// are not modified because the signature covers the historical assistant content.
func mergeMidArraySystemMessages(request *ClaudeMessagesRequest) {
	if request == nil || len(request.Messages) == 0 {
		return
	}

	foundSystem := false
	for _, msg := range request.Messages {
		if msg.Role == "system" {
			foundSystem = true
			break
		}
	}
	if !foundSystem {
		return
	}

	kept := make([]relaymodel.ClaudeMessage, 0, len(request.Messages))
	var pending []string
	for _, msg := range request.Messages {
		if msg.Role == "system" {
			if text := strings.TrimSpace(relayadaptor.ExtractClaudeContentText(msg.Content)); text != "" {
				pending = append(pending, text)
			}
			continue
		}

		if len(pending) > 0 {
			text := strings.Join(pending, systemMergeSeparator)
			if hasSignedClaudeThinking(msg) {
				kept = append(kept, relaymodel.ClaudeMessage{Role: "user", Content: text})
			} else {
				prependClaudeText(&msg, text)
			}
			pending = nil
		}
		kept = append(kept, msg)
	}

	if len(pending) > 0 {
		text := strings.Join(pending, systemMergeSeparator)
		if len(kept) == 0 || hasSignedClaudeThinking(kept[len(kept)-1]) {
			kept = append(kept, relaymodel.ClaudeMessage{Role: "user", Content: text})
		} else {
			appendClaudeText(&kept[len(kept)-1], text)
		}
	}

	if len(kept) == 0 {
		kept = append(kept, relaymodel.ClaudeMessage{Role: "user", Content: " "})
	}

	request.Messages = kept
}

// prependClaudeText inserts text before a Claude message's existing content.
// It preserves string or block-array content shape and returns no value.
func prependClaudeText(msg *relaymodel.ClaudeMessage, text string) {
	switch content := msg.Content.(type) {
	case string:
		if content == "" {
			msg.Content = text
		} else {
			msg.Content = text + systemMergeSeparator + content
		}
	case []any:
		block := map[string]any{"type": "text", "text": text}
		msg.Content = append([]any{block}, content...)
	default:
		if existing := strings.TrimSpace(relayadaptor.ExtractClaudeContentText(msg.Content)); existing != "" {
			msg.Content = text + systemMergeSeparator + existing
		} else {
			msg.Content = text
		}
	}
}

// appendClaudeText adds text after a Claude message's existing content. It
// preserves string or block-array content shape and returns no value.
func appendClaudeText(msg *relaymodel.ClaudeMessage, text string) {
	switch content := msg.Content.(type) {
	case string:
		if content == "" {
			msg.Content = text
		} else {
			msg.Content = content + systemMergeSeparator + text
		}
	case []any:
		block := map[string]any{"type": "text", "text": text}
		msg.Content = append(append([]any{}, content...), block)
	default:
		if existing := strings.TrimSpace(relayadaptor.ExtractClaudeContentText(msg.Content)); existing != "" {
			msg.Content = existing + systemMergeSeparator + text
		} else {
			msg.Content = text
		}
	}
}

// hasSignedClaudeThinking reports whether msg contains a thinking or
// redacted_thinking block with a non-empty signature. It returns true only for
// assistant messages because those are the replayed signed turns that must remain
// byte-stable for native Claude-family upstreams.
func hasSignedClaudeThinking(msg relaymodel.ClaudeMessage) bool {
	if msg.Role != "assistant" {
		return false
	}

	blocks, ok := msg.Content.([]any)
	if !ok {
		return false
	}
	for _, block := range blocks {
		blockMap, ok := block.(map[string]any)
		if !ok {
			continue
		}
		blockType, _ := blockMap["type"].(string)
		normalizedType := strings.ToLower(strings.TrimSpace(blockType))
		if normalizedType != "thinking" && normalizedType != "redacted_thinking" {
			continue
		}
		signature, _ := blockMap["signature"].(string)
		if strings.TrimSpace(signature) != "" {
			return true
		}
	}
	return false
}
