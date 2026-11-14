package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	dbmodel "github.com/Laisky/one-api/model"
	nativegemini "github.com/Laisky/one-api/relay/adaptor/gemini"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/vertexai"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/model"
)

// relayVertexPublicChat executes the public Chat controller against a synthetic native provider and returns public output and the captured upstream request.
func relayVertexPublicChat(t *testing.T, body, candidate string, stream, optIn bool, captureMode ...bool) ([]byte, []byte) {
	t.Helper()
	xaiVideoSetup(t, providerMatrixBalance, false)
	type observation struct {
		path string
		body []byte
		err  error
	}
	observed := make(chan observation, 1)
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		raw, err := io.ReadAll(r.Body)
		observed <- observation{r.URL.Path, raw, err}
		if stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: "+candidate+"\n\ndata: [DONE]\n\n")
		} else {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, candidate)
		}
	}))
	defer server.Close()
	previous := client.HTTPClient
	client.HTTPClient = server.Client()
	defer func() { client.HTTPClient = previous }()
	if stream {
		body = `{"stream":true,` + body[1:]
	}
	c, w, _ := protocolContext(t, channeltype.VertextAI, "gemini-2.5-flash", "/v1/chat/completions", body, server.URL, providerMatrixBalance, 1, false, &dbmodel.ModelConfigLocal{Ratio: 1, CompletionRatio: 2})
	c.Set(ctxkey.Config, dbmodel.ChannelConfig{VertexAIProjectID: providerMatrixVertexProject})
	if optIn {
		c.Request.Header.Set("X-OneAPI-Gemini-Parts", "v1")
	}
	key := fmt.Sprintf("vertexai-token-%d", fallbackChannelID)
	vertexai.Cache.Set(key, providerMatrixVertexToken, time.Minute)
	defer vertexai.Cache.Delete(key)
	var capture *responseCaptureWriter
	publicWriter := c.Writer
	if len(captureMode) > 0 && captureMode[0] {
		capture = newResponseCaptureWriter(c.Writer)
		c.Writer = capture
	}
	apiErr := RelayTextHelper(c)
	drainCriticalTasks(t)
	require.Nil(t, apiErr)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.EqualValues(t, 1, calls.Load())
	var got observation
	select {
	case got = <-observed:
	default:
		t.Fatal("native dispatch was not observed")
	}
	require.NoError(t, got.err)
	suffix := ":generateContent"
	if stream {
		suffix = ":streamGenerateContent"
	}
	require.True(t, strings.HasSuffix(got.path, suffix), got.path)
	if optIn {
		require.Equal(t, "v1", c.Writer.Header().Get("X-OneAPI-Gemini-Parts"))
		require.Contains(t, strings.ToLower(strings.Join(c.Writer.Header().Values("Vary"), ",")), "x-oneapi-gemini-parts")
	}
	responseBody := w.Body.Bytes()
	if capture != nil {
		responseBody = capture.BodyBytes()
		c.Writer = publicWriter
	}
	return append([]byte(nil), responseBody...), got.body
}

// publicChatFrames returns each public Chat JSON object from buffered or SSE output.
func publicChatFrames(t *testing.T, body []byte, stream bool) [][]byte {
	t.Helper()
	if !stream {
		return [][]byte{body}
	}
	var frames [][]byte
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		frames = append(frames, []byte(data))
	}
	require.NotEmpty(t, frames)
	return frames
}

// TestVertexPublicChatDefaultStrictContent verifies ordinary public text/tool-enabled Chat clients receive string-or-null content in JSON and SSE.
func TestVertexPublicChatDefaultStrictContent(t *testing.T) {
	const candidate = `{"candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"hello"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":7,"totalTokenCount":18}}`
	const request = `{"model":"alias","messages":[{"role":"user","content":"begin"}],"tools":[{"type":"function","function":{"name":"one","parameters":{"type":"object"}}}]}`
	for _, stream := range []bool{false, true} {
		name := "json"
		if stream {
			name = "sse"
		}
		t.Run(name, func(t *testing.T) {
			body, _ := relayVertexPublicChat(t, request, candidate, stream, false)
			text := ""
			for _, frame := range publicChatFrames(t, body, stream) {
				var strict struct {
					Choices []struct {
						Message struct {
							Content *string `json:"content"`
						} `json:"message"`
						Delta struct {
							Content *string `json:"content"`
						} `json:"delta"`
					} `json:"choices"`
				}
				require.NoError(t, json.Unmarshal(frame, &strict), "ordinary Chat content must be string or null")
				for _, choice := range strict.Choices {
					if choice.Message.Content != nil {
						text += *choice.Message.Content
					}
					if choice.Delta.Content != nil {
						text += *choice.Delta.Content
					}
				}
			}
			require.Equal(t, "hello", text)
		})
	}
}

// collectVertexPublicAssistant assembles public tool deltas and positional text parts without discarding opaque per-call metadata.
func collectVertexPublicAssistant(t *testing.T, body []byte, stream bool) model.Message {
	t.Helper()
	output := model.Message{Role: "assistant"}
	var text strings.Builder
	var parts []model.MessageContent
	tools := map[int]*model.Tool{}
	var order []int
	for _, frame := range publicChatFrames(t, body, stream) {
		var response struct {
			Choices []struct {
				Message model.Message `json:"message"`
				Delta   model.Message `json:"delta"`
			} `json:"choices"`
		}
		require.NoError(t, json.Unmarshal(frame, &response))
		for _, choice := range response.Choices {
			message := choice.Message
			if stream {
				message = choice.Delta
			}
			if message.ExtraContent != nil {
				output.ExtraContent = message.ExtraContent
			}
			if message.IsStringContent() {
				text.WriteString(message.StringContent())
			} else if message.Content != nil {
				decoded, err := message.ParseContentChecked()
				require.NoError(t, err)
				parts = append(parts, decoded...)
			}
			for i, call := range message.ToolCalls {
				index := i
				if call.Index != nil {
					index = *call.Index
				}
				current := tools[index]
				if current == nil {
					current = &model.Tool{Type: "function", Function: &model.Function{}}
					tools[index] = current
					order = append(order, index)
				}
				if call.Id != "" {
					current.Id = call.Id
				}
				if call.ExtraContent != nil {
					current.ExtraContent = call.ExtraContent
				}
				if call.Function != nil {
					if call.Function.Name != "" {
						current.Function.Name = call.Function.Name
					}
					if arguments, ok := call.Function.Arguments.(string); ok {
						previous, _ := current.Function.Arguments.(string)
						current.Function.Arguments = previous + arguments
					}
				}
			}
		}
	}
	if len(parts) > 0 {
		output.Content = parts
	} else if text.Len() > 0 {
		output.Content = text.String()
	}
	for _, index := range order {
		output.ToolCalls = append(output.ToolCalls, *tools[index])
	}
	return output
}

// TestVertexPublicChatToolOnlyReplay verifies default public Chat retains tool IDs, arguments and per-call signatures without positional array extensions.
func TestVertexPublicChatToolOnlyReplay(t *testing.T) {
	const candidate = `{"candidates":[{"index":0,"content":{"role":"model","parts":[{"functionCall":{"name":"one","args":{"kind":"note"}},"thoughtSignature":"call-one-sig"},{"functionCall":{"name":"two","args":{}},"thoughtSignature":"call-two-sig"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":7,"totalTokenCount":18}}`
	for _, stream := range []bool{false, true} {
		name := "json"
		if stream {
			name = "sse"
		}
		t.Run(name, func(t *testing.T) {
			body, _ := relayVertexPublicChat(t, vertexPublicToolRequest, candidate, stream, false)
			requirePublicChatScalarFrames(t, body, stream)
			assistant := collectVertexPublicAssistant(t, body, stream)
			require.Len(t, assistant.ToolCalls, 2)
			for i, call := range assistant.ToolCalls {
				require.NotEmpty(t, call.Id)
				require.Equal(t, []string{"one", "two"}[i], call.Function.Name)
				require.Equal(t, []string{"call-one-sig", "call-two-sig"}[i], model.ToolThoughtSignature(call))
				_, positioned := model.GeminiPartIndex(call.ExtraContent)
				require.False(t, positioned, "default public tool-only Chat must not retain gateway text positions")
			}
			require.NotEqual(t, assistant.ToolCalls[0].Id, assistant.ToolCalls[1].Id)
			require.JSONEq(t, `{"kind":"note"}`, assistant.ToolCalls[0].Function.Arguments.(string))
			require.JSONEq(t, `{}`, assistant.ToolCalls[1].Function.Arguments.(string))
			native := replayVertexPublicAssistant(t, assistant, false)
			require.Len(t, native, 2)
			require.Equal(t, "call-one-sig", native[0]["thoughtSignature"])
			require.Equal(t, "call-two-sig", native[1]["thoughtSignature"])
			require.Equal(t, map[string]any{"kind": "note"}, native[0]["functionCall"].(map[string]any)["args"])
		})
	}
}

// vertexPublicToolRequest declares the two functions exercised by public Chat continuation tests.
const vertexPublicToolRequest = `{"model":"alias","messages":[{"role":"user","content":"begin"}],"tools":[{"type":"function","function":{"name":"one","parameters":{"type":"object"}}},{"type":"function","function":{"name":"two","parameters":{"type":"object"}}}]}`

// requirePublicChatScalarFrames verifies a strict text/tool Chat client can decode every public message or delta.
func requirePublicChatScalarFrames(t *testing.T, body []byte, stream bool) {
	t.Helper()
	for _, frame := range publicChatFrames(t, body, stream) {
		var strict struct {
			Choices []struct {
				Message struct {
					Content *string `json:"content"`
				} `json:"message"`
				Delta struct {
					Content *string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		require.NoError(t, json.Unmarshal(frame, &strict), "default public Chat must expose string-or-null content")
	}
}

// replayVertexPublicAssistant sends the exact public assistant and reversed tool results through a second real native dispatch.
func replayVertexPublicAssistant(t *testing.T, assistant model.Message, optIn bool) []map[string]any {
	t.Helper()
	messages := []model.Message{assistant}
	for i := len(assistant.ToolCalls) - 1; i >= 0; i-- {
		call := assistant.ToolCalls[i]
		messages = append(messages, model.Message{Role: "tool", ToolCallId: call.Id, Content: "result-" + call.Function.Name})
	}
	raw, err := json.Marshal(map[string]any{"model": "alias", "messages": messages})
	require.NoError(t, err)
	const done = `{"candidates":[{"content":{"role":"model","parts":[{"text":"done"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":7,"totalTokenCount":18}}`
	_, sent := relayVertexPublicChat(t, string(raw), done, false, optIn)
	var native struct {
		Contents []struct {
			Role  string           `json:"role"`
			Parts []map[string]any `json:"parts"`
		} `json:"contents"`
	}
	require.NoError(t, json.Unmarshal(sent, &native))
	var parts []map[string]any
	var responses []map[string]any
	for _, content := range native.Contents {
		if content.Role == "model" {
			parts = append(parts, content.Parts...)
		}
		for _, part := range content.Parts {
			if response, ok := part["functionResponse"].(map[string]any); ok {
				responses = append(responses, response)
			}
		}
	}
	require.Len(t, responses, len(assistant.ToolCalls))
	for i, response := range responses {
		name := assistant.ToolCalls[i].Function.Name
		require.Equal(t, name, response["name"], "reversed results must pair with the original call IDs")
		require.Equal(t, map[string]any{"output": "result-" + name}, response["response"])
	}
	return parts
}

// TestVertexPublicChatPartsOptInReplay verifies explicit rich-part Chat opt-in preserves mixed signed and empty text/tool ordering in JSON and SSE.
func TestVertexPublicChatPartsOptInReplay(t *testing.T) {
	const candidate = `{"candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"thinking","thought":true,"thoughtSignature":"text-sig"},{"functionCall":{"name":"one","args":{"kind":"note"}},"thoughtSignature":"call-sig"},{"text":"","thoughtSignature":"empty-sig"},{"functionCall":{"name":"two","args":{}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":7,"totalTokenCount":18}}`
	for _, stream := range []bool{false, true} {
		name := "json"
		if stream {
			name = "sse"
		}
		t.Run(name, func(t *testing.T) {
			body, _ := relayVertexPublicChat(t, vertexPublicToolRequest, candidate, stream, true)
			assistant := collectVertexPublicAssistant(t, body, stream)
			parts, err := assistant.ParseContentChecked()
			require.NoError(t, err)
			require.Len(t, parts, 2)
			require.NotNil(t, parts[1].Text)
			require.Empty(t, *parts[1].Text)
			require.Len(t, assistant.ToolCalls, 2)
			native := replayVertexPublicAssistant(t, assistant, true)
			require.Len(t, native, 4)
			require.Equal(t, "thinking", native[0]["text"])
			require.Equal(t, true, native[0]["thought"])
			require.Equal(t, "text-sig", native[0]["thoughtSignature"])
			require.Equal(t, "one", native[1]["functionCall"].(map[string]any)["name"])
			require.Equal(t, "call-sig", native[1]["thoughtSignature"])
			require.Equal(t, "", native[2]["text"])
			require.Equal(t, "empty-sig", native[2]["thoughtSignature"])
			require.Equal(t, "two", native[3]["functionCall"].(map[string]any)["name"])
			require.NotContains(t, native[3], "thoughtSignature")
		})
	}
}

// TestVertexPublicChatDefaultSignedText verifies a sole signed text part uses scalar Chat content and message-level metadata without losing replay.
func TestVertexPublicChatDefaultSignedText(t *testing.T) {
	const candidate = `{"candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"hello","thoughtSignature":"text-only-sig"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":7,"totalTokenCount":18}}`
	for _, stream := range []bool{false, true} {
		name := "json"
		if stream {
			name = "sse"
		}
		t.Run(name, func(t *testing.T) {
			body, _ := relayVertexPublicChat(t, vertexPublicToolRequest, candidate, stream, false)
			requirePublicChatScalarFrames(t, body, stream)
			assistant := collectVertexPublicAssistant(t, body, stream)
			require.Equal(t, "hello", assistant.StringContent())
			signature, _ := model.GoogleMetadata(assistant.ExtraContent)
			require.Equal(t, "text-only-sig", signature)
			_, positioned := model.GeminiPartIndex(assistant.ExtraContent)
			require.False(t, positioned)
			native := replayVertexPublicAssistant(t, assistant, false)
			require.Len(t, native, 1)
			require.Equal(t, "hello", native[0]["text"])
			require.Equal(t, "text-only-sig", native[0]["thoughtSignature"])
		})
	}
}

// TestVertexInternalCaptureDefaultWireProjection verifies internal MCP-like capture remains rich without a header before final public scalar projection.
func TestVertexInternalCaptureDefaultWireProjection(t *testing.T) {
	const candidate = `{"candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"thinking","thought":true,"thoughtSignature":"text-sig"},{"functionCall":{"name":"one","args":{}},"thoughtSignature":"call-sig"},{"text":"","thoughtSignature":"empty-sig"},{"functionCall":{"name":"two","args":{}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":7,"totalTokenCount":18}}`
	captured, _ := relayVertexPublicChat(t, vertexPublicToolRequest, candidate, false, false, true)
	var rich openai.TextResponse
	require.NoError(t, json.Unmarshal(captured, &rich))
	require.Len(t, rich.Choices, 1)
	parts, err := rich.Choices[0].Message.ParseContentChecked()
	require.NoError(t, err)
	require.Len(t, parts, 2)
	require.Equal(t, "empty-sig", parts[1].ExtraContent.Google.ThoughtSignature)
	before, err := json.Marshal(rich)
	require.NoError(t, err)
	c, _ := newBridgeTestContext(t)
	projected := nativegemini.ChatResponseForWire(c, &rich)
	wire, err := json.Marshal(projected)
	require.NoError(t, err)
	requirePublicChatScalarFrames(t, wire, false)
	require.Equal(t, "thinking", projected.Choices[0].Message.StringContent())
	require.Equal(t, "call-sig", model.ToolThoughtSignature(projected.Choices[0].Message.ToolCalls[0]))
	for _, call := range projected.Choices[0].Message.ToolCalls {
		_, positioned := model.GeminiPartIndex(call.ExtraContent)
		require.False(t, positioned)
	}
	after, err := json.Marshal(rich)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after), "public projection must not alter internal rich capture")
}
