package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
)

// fallbackWireTool is one upstream tool definition, normalized across Chat
// Completions ({type,function:{...}}) and Claude Messages ({name,input_schema}).
type fallbackWireTool struct {
	Name        string
	Description string
	Schema      json.RawMessage
}

// fallbackWireCall is one upstream assistant tool call (Chat tool_calls entry
// or Claude tool_use block) with its JSON arguments.
type fallbackWireCall struct {
	ID        string
	Name      string
	Arguments string
}

// fallbackWireResult is one upstream tool result (Chat tool message or Claude
// tool_result block) paired to a call ID.
type fallbackWireResult struct {
	ID      string
	Content string
}

// fallbackWireTurn is one upstream message. Chat tool messages and Claude user
// messages that only carry tool_result blocks both normalize to role "tool".
type fallbackWireTurn struct {
	Role    string
	Text    string
	Calls   []fallbackWireCall
	Results []fallbackWireResult
}

// fallbackWire is the normalized upstream request of a Responses fallback.
type fallbackWire struct {
	Tools     []fallbackWireTool
	Turns     []fallbackWireTurn
	Reasoning string
}

// fallbackWireText decodes a JSON string or text-part array into plain text.
// Parameters: raw is the content value. Returns the concatenated text.
func fallbackWireText(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	require.NoError(t, json.Unmarshal(raw, &parts), "content must be a string or text parts: %s", raw)
	var b strings.Builder
	for _, part := range parts {
		b.WriteString(part.Text)
	}
	return b.String()
}

// decodeFallbackWire normalizes the upstream request body for channel.
// Parameters: t owns assertions, channel is OpenAICompatible or Anthropic and
// body is the raw upstream request. Returns the normalized request.
func decodeFallbackWire(t *testing.T, channel int, body []byte) fallbackWire {
	t.Helper()
	var wire fallbackWire
	if channel == channeltype.Anthropic {
		var claude struct {
			Tools []struct {
				Name        string          `json:"name"`
				Description string          `json:"description"`
				InputSchema json.RawMessage `json:"input_schema"`
			} `json:"tools"`
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		require.NoError(t, json.Unmarshal(body, &claude))
		for _, tool := range claude.Tools {
			wire.Tools = append(wire.Tools, fallbackWireTool{Name: tool.Name, Description: tool.Description, Schema: tool.InputSchema})
		}
		for _, message := range claude.Messages {
			turn := fallbackWireTurn{Role: message.Role}
			var blocks []struct {
				Type      string          `json:"type"`
				Text      string          `json:"text"`
				ID        string          `json:"id"`
				Name      string          `json:"name"`
				Input     json.RawMessage `json:"input"`
				ToolUseID string          `json:"tool_use_id"`
				Content   json.RawMessage `json:"content"`
			}
			if json.Unmarshal(message.Content, &blocks) != nil {
				turn.Text = fallbackWireText(t, message.Content)
				wire.Turns = append(wire.Turns, turn)
				continue
			}
			for _, block := range blocks {
				switch block.Type {
				case "text":
					turn.Text += block.Text
				case "tool_use":
					turn.Calls = append(turn.Calls, fallbackWireCall{ID: block.ID, Name: block.Name, Arguments: string(block.Input)})
				case "tool_result":
					turn.Results = append(turn.Results, fallbackWireResult{ID: block.ToolUseID, Content: fallbackWireText(t, block.Content)})
				default:
					require.Failf(t, "unexpected Claude content block", "type %q", block.Type)
				}
			}
			if len(turn.Results) > 0 {
				require.Empty(t, turn.Text, "Claude tool results stay in their own user turn")
				turn.Role = "tool"
			}
			wire.Turns = append(wire.Turns, turn)
		}
		return wire
	}
	var chat struct {
		Tools []struct {
			Type     string `json:"type"`
			Function struct {
				Name        string          `json:"name"`
				Description string          `json:"description"`
				Parameters  json.RawMessage `json:"parameters"`
			} `json:"function"`
		} `json:"tools"`
		Messages []struct {
			Role       string          `json:"role"`
			Content    json.RawMessage `json:"content"`
			ToolCallID string          `json:"tool_call_id"`
			ToolCalls  []struct {
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"messages"`
		Reasoning *struct {
			Effort string `json:"effort"`
		} `json:"reasoning"`
	}
	require.NoError(t, json.Unmarshal(body, &chat))
	for _, tool := range chat.Tools {
		require.Equal(t, "function", tool.Type)
		wire.Tools = append(wire.Tools, fallbackWireTool{Name: tool.Function.Name, Description: tool.Function.Description, Schema: tool.Function.Parameters})
	}
	for _, message := range chat.Messages {
		turn := fallbackWireTurn{Role: message.Role}
		if message.Role == "tool" {
			turn.Results = []fallbackWireResult{{ID: message.ToolCallID, Content: fallbackWireText(t, message.Content)}}
		} else {
			turn.Text = fallbackWireText(t, message.Content)
		}
		for _, call := range message.ToolCalls {
			turn.Calls = append(turn.Calls, fallbackWireCall{ID: call.ID, Name: call.Function.Name, Arguments: call.Function.Arguments})
		}
		wire.Turns = append(wire.Turns, turn)
	}
	if chat.Reasoning != nil {
		wire.Reasoning = chat.Reasoning.Effort
	}
	return wire
}

// fallbackWireToolNames returns the upstream tool names in request order.
func fallbackWireToolNames(wire fallbackWire) []string {
	names := make([]string, 0, len(wire.Tools))
	for _, tool := range wire.Tools {
		names = append(names, tool.Name)
	}
	return names
}

// fallbackWireToolByName returns the upstream tool called name. Parameters: t
// owns assertions, wire is the normalized request and name the flat name.
func fallbackWireToolByName(t *testing.T, wire fallbackWire, name string) fallbackWireTool {
	t.Helper()
	for _, tool := range wire.Tools {
		if tool.Name == name {
			return tool
		}
	}
	require.Failf(t, "missing upstream tool", "tool %q not in %v", name, fallbackWireToolNames(wire))
	return fallbackWireTool{}
}

// fallbackUpstreamPath returns the upstream path fallback must call for channel.
func fallbackUpstreamPath(channel int) string {
	if channel == channeltype.Anthropic {
		return "/v1/messages"
	}
	return "/v1/chat/completions"
}

// writeFallbackTextReply answers a fallback upstream request with a plain text
// completion carrying 11 prompt and 7 completion tokens in channel's format.
// Parameters: w is the response writer and channel picks Chat or Claude JSON.
func writeFallbackTextReply(w http.ResponseWriter, channel int) {
	w.Header().Set("Content-Type", "application/json")
	if channel == channeltype.Anthropic {
		_, _ = fmt.Fprint(w, `{"id":"msg_fallback","type":"message","role":"assistant","model":"fallback-fixture","content":[{"type":"text","text":"done"}],"stop_reason":"end_turn","usage":{"input_tokens":11,"output_tokens":7}}`)
		return
	}
	_, _ = fmt.Fprint(w, `{"id":"chatcmpl_fallback","object":"chat.completion","created":1762387200,"model":"fallback-fixture","choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`)
}
