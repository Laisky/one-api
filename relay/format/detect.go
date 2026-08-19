// Package format provides utilities for detecting and handling different API request formats.
// one-api supports three main chat-style API formats:
// - OpenAI Chat Completions (/v1/chat/completions)
// - OpenAI Responses API (/v1/responses)
// - Claude Messages API (/v1/messages)
//
// This package enables automatic detection of the request format based on the payload structure,
// allowing the system to handle requests sent to incorrect endpoints transparently.
package format

import (
	"bytes"
	"encoding/json"

	"github.com/Laisky/errors/v2"
)

// APIFormat represents the detected API format of a request.
type APIFormat int

const (
	// Unknown indicates the format could not be determined.
	Unknown APIFormat = iota
	// ChatCompletion represents OpenAI Chat Completions API format.
	ChatCompletion
	// ResponseAPI represents OpenAI Responses API format.
	ResponseAPI
	// ClaudeMessages represents Claude Messages API format.
	ClaudeMessages
)

// String returns the string representation of the API format.
func (f APIFormat) String() string {
	switch f {
	case ChatCompletion:
		return "chat_completion"
	case ResponseAPI:
		return "response_api"
	case ClaudeMessages:
		return "claude_messages"
	default:
		return "unknown"
	}
}

// Endpoint returns the canonical endpoint path for the API format.
func (f APIFormat) Endpoint() string {
	switch f {
	case ChatCompletion:
		return "/v1/chat/completions"
	case ResponseAPI:
		return "/v1/responses"
	case ClaudeMessages:
		return "/v1/messages"
	default:
		return ""
	}
}

// requestProbe is a minimal structure used for format detection.
// It only parses the fields necessary to distinguish between formats,
// avoiding the overhead of full request parsing.
type requestProbe struct {
	// ChatCompletion / Claude Messages indicator
	Messages json.RawMessage `json:"messages,omitempty"`

	// Response API indicators
	Input        json.RawMessage `json:"input,omitempty"`
	Instructions *string         `json:"instructions,omitempty"`

	// Response API state selectors — these fields are exclusive to the Responses
	// API and let a stateful request (which may omit input entirely) be detected.
	PreviousResponseId *string         `json:"previous_response_id,omitempty"`
	Conversation       json.RawMessage `json:"conversation,omitempty"`
	Prompt             json.RawMessage `json:"prompt,omitempty"`

	// Response API specific
	MaxOutputTokens *int `json:"max_output_tokens,omitempty"`

	// Tool definitions for distinguishing Claude vs OpenAI
	Tools json.RawMessage `json:"tools,omitempty"`
}

// messageProbe is used to inspect the structure of messages array entries.
type messageProbe struct {
	Content json.RawMessage `json:"content,omitempty"`
}

// toolProbe is used to inspect tool definitions.
type toolProbe struct {
	// OpenAI uses "function" type with nested "function" object
	Type     string          `json:"type,omitempty"`
	Function json.RawMessage `json:"function,omitempty"`

	// Claude uses "name" and "input_schema" directly
	Name        string          `json:"name,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

// contentBlockProbe is used to check Claude-specific content block types.
type contentBlockProbe struct {
	Type string `json:"type,omitempty"`
}

// DetectFormat analyzes a JSON request body and determines its API format.
// It returns the detected format and any error encountered during parsing.
//
// IMPORTANT: This function is intentionally conservative. It only returns a
// definite format when there is 100% certainty that the request cannot be
// valid for other formats. If there's any ambiguity (e.g., a simple messages
// array that could be either ChatCompletion or Claude Messages), it returns
// Unknown to avoid breaking backward compatibility.
//
// Detection rules (only when unambiguous):
// 1. Response API: has "input" field WITHOUT "messages" - this is exclusive to Response API
// 2. Claude Messages: has Claude-ONLY features (tool_use/tool_result content, input_schema tools)
// 3. Unknown: any ambiguous case (including simple messages that could be either format)
//
// The function is designed to be fast and only parses the minimum required
// fields to make a determination.
func DetectFormat(body []byte) (APIFormat, error) {
	if len(body) == 0 {
		return Unknown, errors.New("empty request body")
	}

	var probe requestProbe
	if err := json.Unmarshal(body, &probe); err != nil {
		return Unknown, errors.Wrap(err, "failed to parse request body for format detection")
	}

	// ==========================================================================
	// Response API detection (high confidence)
	// Response API uses "input" instead of "messages" - this is unambiguous
	// ==========================================================================

	// If we have "input" without "messages", this is definitely Response API
	// Note: Response API's "input" is for the main content, not embeddings input
	if len(probe.Input) > 0 && len(probe.Messages) == 0 {
		return ResponseAPI, nil
	}

	// max_output_tokens without messages is Response API specific
	if probe.MaxOutputTokens != nil && len(probe.Messages) == 0 {
		return ResponseAPI, nil
	}

	// instructions without messages is Response API specific
	if probe.Instructions != nil && len(probe.Messages) == 0 {
		return ResponseAPI, nil
	}

	// State selectors are exclusive to the Responses API. A stateful request may
	// omit input entirely — continuing a chain via previous_response_id, resuming
	// a conversation, or running a prompt template — so these must be recognized
	// even when no input/messages discriminator is present.
	if len(probe.Messages) == 0 {
		if probe.PreviousResponseId != nil {
			return ResponseAPI, nil
		}
		if isNonEmptyJSONValue(probe.Conversation) {
			return ResponseAPI, nil
		}
		if isResponseAPIPromptObject(probe.Prompt) {
			return ResponseAPI, nil
		}
	}

	// ==========================================================================
	// If no messages and no Response API indicators, we can't determine format
	// ==========================================================================
	if len(probe.Messages) == 0 {
		return Unknown, nil
	}

	// ==========================================================================
	// We have messages - check for Claude-EXCLUSIVE features
	// IMPORTANT: Many fields overlap between Claude and ChatCompletion:
	// - messages, model, max_tokens, temperature, top_p, stream, tools (with function type)
	// - system field: Claude uses top-level, but some clients might send it incorrectly
	//
	// We only identify as Claude when we see features that CANNOT be ChatCompletion
	// ==========================================================================

	// Check for Claude-specific content blocks in messages
	// tool_use, tool_result, thinking are EXCLUSIVE to Claude Messages API
	if hasClaudeContentBlocks(probe.Messages) {
		return ClaudeMessages, nil
	}

	// Check for Claude-specific tool format (input_schema at tool level)
	// OpenAI tools use type="function" with nested function.parameters
	// Claude tools use name + input_schema directly at tool level
	if len(probe.Tools) > 0 && isClaudeToolFormat(probe.Tools) {
		return ClaudeMessages, nil
	}

	// ==========================================================================
	// AMBIGUOUS CASES - Return Unknown to preserve backward compatibility
	// ==========================================================================
	// The following could be either ChatCompletion or Claude Messages:
	// - Simple messages array with role/content
	// - messages + max_tokens
	// - messages + system (some clients incorrectly send system as top-level)
	// - messages + standard tools (type=function with function.parameters)
	//
	// We do NOT try to distinguish these - let the endpoint handle them as-is

	return Unknown, nil
}

// isNonEmptyJSONValue reports whether a raw JSON field carries a meaningful,
// non-null value (used for the conversation selector, which may be a string or
// an object).
func isNonEmptyJSONValue(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return false
	}
	return !bytes.Equal(trimmed, []byte("null"))
}

// isResponseAPIPromptObject reports whether the prompt field is a Responses API
// prompt-template object ({"id": ...}). It deliberately rejects the legacy
// Completions-style string prompt so that a bare string prompt is never
// misclassified as a Responses request.
func isResponseAPIPromptObject(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return false
	}
	return trimmed[0] == '{'
}

// isClaudeToolFormat checks if the tools array uses Claude's format (input_schema).
func isClaudeToolFormat(toolsRaw json.RawMessage) bool {
	var tools []toolProbe
	if err := json.Unmarshal(toolsRaw, &tools); err != nil {
		return false
	}

	for _, tool := range tools {
		// Claude tools have input_schema directly on the tool object
		// and don't wrap the function in a nested "function" field
		if len(tool.InputSchema) > 0 && tool.Name != "" {
			return true
		}

		// OpenAI tools have type="function" with a nested function object.
		if tool.Type == "function" && len(tool.Function) > 0 {
			// Only input_schema affects detection. Skipping parameters avoids copying
			// potentially large OpenAI JSON schemas into an unused RawMessage.
			var fnProbe struct {
				InputSchema json.RawMessage `json:"input_schema,omitempty"`
			}
			if err := json.Unmarshal(tool.Function, &fnProbe); err == nil {
				if len(fnProbe.InputSchema) > 0 {
					return true
				}
			}
		}
	}

	return false
}

// hasClaudeContentBlocks checks if any message contains Claude-specific content block types.
func hasClaudeContentBlocks(messagesRaw json.RawMessage) bool {
	var messages []messageProbe
	if err := json.Unmarshal(messagesRaw, &messages); err != nil {
		return false
	}

	for _, msg := range messages {
		// Claude-specific content blocks can only appear in an array. Most text
		// messages are JSON strings, so skip the failed array decode on that hot path.
		content := bytes.TrimSpace(msg.Content)
		if len(content) == 0 || content[0] != '[' {
			continue
		}

		var contentArray []contentBlockProbe
		if err := json.Unmarshal(content, &contentArray); err == nil {
			for _, block := range contentArray {
				// Claude-specific content types
				switch block.Type {
				case "tool_use", "tool_result", "thinking":
					return true
				}
			}
		}
	}

	return false
}

// FormatFromPath returns the expected API format based on the request path.
func FormatFromPath(path string) APIFormat {
	switch {
	case pathMatches(path, "/v1/chat/completions"):
		return ChatCompletion
	case pathMatches(path, "/v1/responses"):
		return ResponseAPI
	case pathMatches(path, "/v1/messages"):
		return ClaudeMessages
	default:
		return Unknown
	}
}

// pathMatches checks if the path starts with the given prefix.
func pathMatches(path, prefix string) bool {
	if len(path) < len(prefix) {
		return false
	}
	return path[:len(prefix)] == prefix
}
