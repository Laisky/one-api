package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/model"
)

// googleTextFixture returns typed Chat text parts with positional opaque metadata, including an empty signed part.
func googleTextFixture(t *testing.T) model.Message {
	t.Helper()
	var message model.Message
	require.NoError(t, json.Unmarshal([]byte(`{"role":"assistant","content":[{"type":"text","text":" first ","extra_content":{"google":{"thought_signature":"sig-a","thought":true}}},{"type":"text","text":"","extra_content":{"google":{"thought_signature":"sig-empty"}}},{"type":"text","text":"last","extra_content":{"google":{"thought_signature":"sig-b"}}}]}`), &message))
	return message
}

// requireGoogleTextReplay verifies gateway output metadata survives native replay without creating Anthropic thinking.
func requireGoogleTextReplay(t *testing.T, output []openai.OutputItem) {
	t.Helper()
	raw, err := json.Marshal(output)
	require.NoError(t, err)
	var request openai.ResponseAPIRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model":"gemini-3.8-flash","input":`+string(raw)+`}`), &request))
	chat, err := openai.ConvertResponseAPIToChatCompletionRequest(&request, true)
	require.NoError(t, err)
	var signatures, texts []string
	for _, message := range chat.Messages {
		require.Nil(t, message.Signature)
		require.Empty(t, message.ClaudeThinkingBlocks)
		parts, err := message.ParseContentChecked()
		require.NoError(t, err)
		for _, part := range parts {
			if part.Text != nil {
				texts = append(texts, *part.Text)
			}
			signature, _ := model.GoogleMetadata(part.ExtraContent)
			signatures = append(signatures, signature)
		}
	}
	require.Equal(t, []string{"sig-a", "sig-empty", "sig-b"}, signatures)
	require.Equal(t, []string{" first ", "", "last"}, texts)
	control, err := openai.ConvertResponseAPIToChatCompletionRequest(&request)
	require.NoError(t, err)
	for _, message := range control.Messages {
		require.Nil(t, message.ExtraContent)
		for _, part := range message.ParseContent() {
			require.Nil(t, part.ExtraContent)
		}
	}
}

// TestResponsesBufferedGoogleTextMetadata verifies signed text boundaries survive buffered Responses conversion and replay.
func TestResponsesBufferedGoogleTextMetadata(t *testing.T) {
	output := buildResponseOutput(logger.Logger, []openai_compatible.TextResponseChoice{{Message: googleTextFixture(t)}}, nil)
	require.Len(t, output, 1)
	require.Len(t, output[0].Content, 3)
	requireGoogleTextReplay(t, output)
}

// TestResponsesStreamGoogleTextMetadata verifies empty signed chunks and terminal state retain positional metadata.
func TestResponsesStreamGoogleTextMetadata(t *testing.T) {
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)
	message := googleTextFixture(t)
	parts, err := message.ParseContentChecked()
	require.NoError(t, err)
	for _, part := range parts {
		bridge.HandleChunk(c, &openai_compatible.ChatCompletionsStreamResponse{Choices: []openai_compatible.ChatCompletionsStreamResponseChoice{{Delta: model.Message{Role: "assistant", Content: []model.MessageContent{part}}}}})
	}
	bridge.HandleDone(c)
	require.NoError(t, bridge.StreamWriteError())
	terminal := bridgeFindEvents(parseBridgeSSE(w.Body.String()), "response.completed")
	require.Len(t, terminal, 1)
	var event openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, terminal[0], &event)
	requireGoogleTextReplay(t, event.Response.Output)
}

// TestResponsesBufferedGoogleStringMetadata verifies message-level carriers retain empty and whitespace signed strings.
func TestResponsesBufferedGoogleStringMetadata(t *testing.T) {
	for _, text := range []string{"", "  signed  "} {
		message := model.Message{Role: "assistant", Content: text, ExtraContent: model.GoogleThoughtSignature("string-sig")}
		output := buildResponseOutput(logger.Logger, []openai_compatible.TextResponseChoice{{Message: message}}, nil)
		require.Len(t, output, 1)
		raw, err := json.Marshal(output)
		require.NoError(t, err)
		require.Contains(t, string(raw), "string-sig")
		require.Equal(t, text, output[0].Content[0].Text)
	}
}

// TestResponsesStreamGoogleMetadataBoundaries verifies unsigned neighbors and secondary choices keep independent content indexes.
func TestResponsesStreamGoogleMetadataBoundaries(t *testing.T) {
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)
	empty := ""
	for _, delta := range []model.Message{
		{Content: "prefix"},
		{Content: []model.MessageContent{{Type: model.ContentTypeText, Text: &empty, ExtraContent: model.GoogleThoughtSignature("empty-boundary")}}},
		{Content: "suffix"},
	} {
		bridge.HandleChunk(c, &openai_compatible.ChatCompletionsStreamResponse{Choices: []openai_compatible.ChatCompletionsStreamResponseChoice{{Index: 0, Delta: delta}}})
	}
	bridge.HandleChunk(c, &openai_compatible.ChatCompletionsStreamResponse{Choices: []openai_compatible.ChatCompletionsStreamResponseChoice{{Index: 1, Delta: googleTextFixture(t)}}})
	bridge.HandleDone(c)
	require.NoError(t, bridge.StreamWriteError())
	terminal := bridgeFindEvents(parseBridgeSSE(w.Body.String()), "response.completed")
	require.Len(t, terminal, 1)
	var event openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, terminal[0], &event)
	require.Len(t, event.Response.Output, 2)
	primary := event.Response.Output[0]
	require.Len(t, primary.Content, 3)
	require.Equal(t, "prefix", primary.Content[0].Text)
	require.Equal(t, "", primary.Content[1].Text)
	signature, _ := model.GoogleMetadata(primary.Content[1].ExtraContent)
	require.Equal(t, "empty-boundary", signature)
	require.Equal(t, "suffix", primary.Content[2].Text)
	requireGoogleTextReplay(t, event.Response.Output[1:])
	done := bridgeFindEvents(parseBridgeSSE(w.Body.String()), "response.content_part.done")
	require.Len(t, done, 6, "every positional part must complete once")
}

// TestResponsesGoogleTextMetadataMergesToolTurn verifies text carriers survive the assistant-turn merger between calls and their outputs.
func TestResponsesGoogleTextMetadataMergesToolTurn(t *testing.T) {
	message := googleTextFixture(t)
	parts, err := json.Marshal(convertMessageContent(message))
	require.NoError(t, err)
	raw := `{"model":"gemini-3.8-flash","input":[{"type":"function_call","call_id":"call_x","name":"lookup","arguments":"{}"},{"type":"message","role":"assistant","content":` + string(parts) + `},{"type":"function_call_output","call_id":"call_x","output":"ok"}]}`
	var request openai.ResponseAPIRequest
	require.NoError(t, json.Unmarshal([]byte(raw), &request))
	chat, err := openai.ConvertResponseAPIToChatCompletionRequest(&request, true)
	require.NoError(t, err)
	require.Len(t, chat.Messages, 2)
	require.Len(t, chat.Messages[0].ToolCalls, 1)
	replayParts, err := chat.Messages[0].ParseContentChecked()
	require.NoError(t, err)
	require.Len(t, replayParts, 3)
	for i, want := range []string{"sig-a", "sig-empty", "sig-b"} {
		got, _ := model.GoogleMetadata(replayParts[i].ExtraContent)
		require.Equal(t, want, got)
	}
}

// TestResponsesGoogleIndexOnlyMetadata verifies unsigned native part positions survive the gateway carrier independently of signatures.
func TestResponsesGoogleIndexOnlyMetadata(t *testing.T) {
	output := buildResponseOutput(logger.Logger, []openai_compatible.TextResponseChoice{{Message: model.Message{Role: "assistant", Content: "unsigned", ExtraContent: model.WithGeminiPartIndex(nil, 0)}}}, nil)
	raw, err := json.Marshal(output)
	require.NoError(t, err)
	var request openai.ResponseAPIRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model":"gemini-3.8-flash","input":`+string(raw)+`}`), &request))
	chat, err := openai.ConvertResponseAPIToChatCompletionRequest(&request, true)
	require.NoError(t, err)
	require.Len(t, chat.Messages, 1)
	parts, err := chat.Messages[0].ParseContentChecked()
	require.NoError(t, err)
	require.Len(t, parts, 1)
	index, ok := model.GeminiPartIndex(parts[0].ExtraContent)
	require.True(t, ok)
	require.Zero(t, index)
}
