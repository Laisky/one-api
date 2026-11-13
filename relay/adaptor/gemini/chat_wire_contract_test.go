package gemini

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/model"
)

// chatWireStrictMessage models the public SDK string-or-null content contract independently of gateway message types.
type chatWireStrictMessage struct {
	Content      *string        `json:"content"`
	ExtraContent map[string]any `json:"extra_content"`
	ToolCalls    []struct {
		ID       string `json:"id"`
		Index    *int   `json:"index"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
		ExtraContent map[string]any `json:"extra_content"`
	} `json:"tool_calls"`
}

// chatWireStrictChoice decodes both actual public response modes without accepting array content.
type chatWireStrictChoice struct {
	Message      chatWireStrictMessage `json:"message"`
	Delta        chatWireStrictMessage `json:"delta"`
	FinishReason *string               `json:"finish_reason"`
}

// renderChatWireFixture invokes the actual public renderer and verifies its converted source request remains unchanged.
func renderChatWireFixture(t *testing.T, parts string, stream bool, header string, vertex, tools bool) (string, *model.Usage) {
	t.Helper()
	c, recorder := signatureTestContext()
	if header != "" {
		c.Request.Header.Set("X-OneAPI-Gemini-Parts", header)
	}
	source := &ChatRequest{}
	if tools {
		source.Tools = []ChatTools{{FunctionDeclarations: []map[string]any{{"name": "lookup", "parametersJsonSchema": map[string]any{"type": "object"}}}}}
	}
	c.Set(ctxkey.ConvertedRequest, source)
	if vertex {
		EnableVertexPartPositions(c)
	}
	before, err := json.Marshal(source)
	require.NoError(t, err)
	body := `{"candidates":[{"index":0,"content":{"role":"model","parts":[` + parts + `]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":7,"candidatesTokenCount":3,"totalTokenCount":10}}`
	upstream := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
	var usage *model.Usage
	if stream {
		upstream.Header.Set("Content-Type", "text/event-stream")
		upstream.Body = io.NopCloser(strings.NewReader("data: " + body + "\n\n"))
		failure, _, receipt := StreamHandler(c, upstream)
		require.Nil(t, failure)
		usage = receipt
		require.Equal(t, 1, strings.Count(recorder.Body.String(), "data: [DONE]"))
	} else {
		failure, receipt := Handler(c, upstream, 7, "gemini-3.8-flash")
		require.Nil(t, failure)
		usage = receipt
	}
	require.NotNil(t, usage)
	require.Equal(t, 10, usage.TotalTokens)
	after, err := json.Marshal(source)
	require.NoError(t, err)
	require.Equal(t, string(before), string(after), "wire projection must not mutate the converted source request")
	return recorder.Body.String(), usage
}

// strictChatWireChoices decodes every actual public SSE chunk or buffered choice using SDK-compatible scalar content.
func strictChatWireChoices(t *testing.T, body string, stream bool) []chatWireStrictChoice {
	t.Helper()
	var result []chatWireStrictChoice
	decode := func(raw string) {
		var envelope struct {
			Choices []chatWireStrictChoice `json:"choices"`
		}
		require.NoError(t, json.Unmarshal([]byte(raw), &envelope), "public Chat content must decode as *string: %s", raw)
		result = append(result, envelope.Choices...)
	}
	if !stream {
		decode(body)
		return result
	}
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		raw := strings.TrimPrefix(line, "data: ")
		if raw != "[DONE]" {
			decode(raw)
		}
	}
	return result
}

// TestGeminiPublicChatScalarWireContract keeps default public text-only Chat compatible with strict SDK decoders.
func TestGeminiPublicChatScalarWireContract(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, tc := range []struct {
			name, parts, text, signature string
			tool, metadataOnly           bool
		}{
			{"tool_enabled_plaintext", `{"text":"hello"}`, "hello", "", false, false},
			{"mixed_tools", `{"text":"checking","thoughtSignature":"dGV4dC1zaWc="},{"functionCall":{"name":"lookup","args":{"q":"a"}},"thoughtSignature":"dG9vbC1zaWc="}`, "checking", "dGV4dC1zaWc=", true, false},
			{"signed_empty", `{"thoughtSignature":"ZW1wdHk="}`, "", "ZW1wdHk=", false, true},
			{"multiple_signed_text", `{"text":"first","thoughtSignature":"c2lnLWE="},{"text":"second","thoughtSignature":"c2lnLWI="}`, "firstsecond", "", false, false},
		} {
			t.Run(fmt.Sprint(stream)+"/"+tc.name, func(t *testing.T) {
				body, _ := renderChatWireFixture(t, tc.parts, stream, "", true, true)
				choices := strictChatWireChoices(t, body, stream)
				require.NotEmpty(t, choices)
				var text strings.Builder
				var calls int
				finish := ""
				var metadata map[string]any
				for _, choice := range choices {
					message := choice.Message
					if stream {
						message = choice.Delta
					}
					if message.Content != nil {
						text.WriteString(*message.Content)
					}
					if message.ExtraContent != nil {
						metadata = message.ExtraContent
					}
					for _, call := range message.ToolCalls {
						calls++
						require.NotEmpty(t, call.ID)
						require.Equal(t, "lookup", call.Function.Name)
						require.JSONEq(t, `{"q":"a"}`, call.Function.Arguments)
						require.Equal(t, map[string]any{"thought_signature": "dG9vbC1zaWc="}, call.ExtraContent["google"])
						if stream {
							require.NotNil(t, call.Index)
							require.Zero(t, *call.Index)
						}
					}
					if choice.FinishReason != nil {
						finish = *choice.FinishReason
					}
				}
				require.Equal(t, tc.text, text.String())
				require.NotContains(t, body, "gemini_part_index", "default public wire must not leak unreplayable positional indices")
				if tc.tool {
					require.Equal(t, 1, calls)
					require.Equal(t, "tool_calls", finish)
				} else {
					require.Zero(t, calls)
					require.Equal(t, "stop", finish)
				}
				if tc.signature != "" {
					require.Equal(t, map[string]any{"thought_signature": tc.signature}, metadata["google"])
				} else {
					require.Nil(t, metadata, "multiple text signatures must not be reassigned to one flattened part")
				}
				if tc.metadataOnly {
					require.Equal(t, map[string]any{"gemini_text_present": false}, metadata["oneapi"])
				}
			})
		}
	}
}

// TestGeminiPublicChatRichOptInRetainsArrays verifies explicit opt-in preserves existing lossless arrays and metadata.
func TestGeminiPublicChatRichOptInRetainsArrays(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			parts := `{"text":"first","thoughtSignature":"c2lnLWE="},{"text":"","thoughtSignature":"c2lnLWI="},{"functionCall":{"name":"lookup","args":{"q":"a"}},"thoughtSignature":"dG9vbC1zaWc="}`
			body, _ := renderChatWireFixture(t, parts, stream, "v1", true, true)
			var textParts []map[string]any
			decode := func(raw string) {
				var envelope struct {
					Choices []struct {
						Message struct {
							Content []map[string]any `json:"content"`
						} `json:"message"`
						Delta struct {
							Content []map[string]any `json:"content"`
						} `json:"delta"`
					} `json:"choices"`
				}
				require.NoError(t, json.Unmarshal([]byte(raw), &envelope))
				for _, choice := range envelope.Choices {
					if stream {
						textParts = append(textParts, choice.Delta.Content...)
					} else {
						textParts = append(textParts, choice.Message.Content...)
					}
				}
			}
			if stream {
				for _, line := range strings.Split(body, "\n") {
					if strings.HasPrefix(line, "data: ") && !strings.HasSuffix(line, "[DONE]") {
						decode(strings.TrimPrefix(line, "data: "))
					}
				}
			} else {
				decode(body)
			}
			require.Len(t, textParts, 2)
			require.Equal(t, "first", textParts[0]["text"])
			require.Equal(t, "", textParts[1]["text"])
			for i, sig := range []string{"c2lnLWE=", "c2lnLWI="} {
				extra := textParts[i]["extra_content"].(map[string]any)
				require.Equal(t, sig, extra["google"].(map[string]any)["thought_signature"])
				require.Equal(t, float64(i), extra["oneapi"].(map[string]any)["gemini_part_index"])
			}
			require.Contains(t, body, "dG9vbC1zaWc=")
		})
	}
}

// TestGeminiPublicChatExistingControls leaves unsigned non-Vertex text and preexisting image-array output unchanged.
func TestGeminiPublicChatExistingControls(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream)+"/unsigned_nonvertex", func(t *testing.T) {
			body, _ := renderChatWireFixture(t, `{"text":"legacy"}`, stream, "", false, false)
			choices := strictChatWireChoices(t, body, stream)
			require.Len(t, choices, 1)
			message := choices[0].Message
			if stream {
				message = choices[0].Delta
			}
			require.NotNil(t, message.Content)
			require.Equal(t, "legacy", *message.Content)
			require.Nil(t, message.ExtraContent)
		})
		t.Run(fmt.Sprint(stream)+"/image_extension", func(t *testing.T) {
			body, _ := renderChatWireFixture(t, `{"text":"image"},{"inlineData":{"mimeType":"image/png","data":"AA=="}}`, stream, "", false, false)
			var content []map[string]any
			decode := func(raw string) {
				var envelope struct {
					Choices []struct {
						Message struct {
							Content []map[string]any `json:"content"`
						} `json:"message"`
						Delta struct {
							Content []map[string]any `json:"content"`
						} `json:"delta"`
					} `json:"choices"`
				}
				require.NoError(t, json.Unmarshal([]byte(raw), &envelope))
				for _, choice := range envelope.Choices {
					if stream {
						content = append(content, choice.Delta.Content...)
					} else {
						content = append(content, choice.Message.Content...)
					}
				}
			}
			if stream {
				for _, line := range strings.Split(body, "\n") {
					if strings.HasPrefix(line, "data: ") && !strings.HasSuffix(line, "[DONE]") {
						decode(strings.TrimPrefix(line, "data: "))
					}
				}
			} else {
				decode(body)
			}
			require.Len(t, content, 2)
			require.Equal(t, "image_url", content[1]["type"])
			require.Equal(t, "data:image/png;base64,AA==", content[1]["image_url"].(map[string]any)["url"])
		})
	}
}

// chatWireInternalCapture marks a gateway-owned intermediate writer without changing public request negotiation.
type chatWireInternalCapture struct{ gin.ResponseWriter }

// PreserveNativeChatParts retains native metadata for the subsequent internal protocol renderer.
func (chatWireInternalCapture) PreserveNativeChatParts() bool { return true }

// TestGeminiChatWireProjectionPreservesInternalSource covers JSON-decoded MCP-like parts and nonmutating public projection.
func TestGeminiChatWireProjectionPreservesInternalSource(t *testing.T) {
	var source openai.TextResponse
	require.NoError(t, json.Unmarshal([]byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":[{"type":"text","text":"one","extra_content":{"google":{"thought_signature":"text-sig"},"oneapi":{"gemini_part_index":0}}}],"tool_calls":[{"id":"call-one","type":"function","function":{"name":"lookup","arguments":"{}"},"extra_content":{"google":{"thought_signature":"tool-sig"},"oneapi":{"gemini_part_index":1}}}]},"finish_reason":"tool_calls"}]}`), &source))
	before, err := json.Marshal(source)
	require.NoError(t, err)
	c, _ := signatureTestContext()
	wire := ChatResponseForWire(c, &source)
	require.NotNil(t, wire)
	encoded, err := json.Marshal(wire)
	require.NoError(t, err)
	choices := strictChatWireChoices(t, string(encoded), false)
	require.Len(t, choices, 1)
	require.NotNil(t, choices[0].Message.Content)
	require.Equal(t, "one", *choices[0].Message.Content)
	require.Equal(t, map[string]any{"thought_signature": "text-sig"}, choices[0].Message.ExtraContent["google"])
	require.NotContains(t, string(encoded), "gemini_part_index")
	require.Equal(t, "call-one", choices[0].Message.ToolCalls[0].ID)
	require.Equal(t, map[string]any{"thought_signature": "tool-sig"}, choices[0].Message.ToolCalls[0].ExtraContent["google"])
	after, err := json.Marshal(source)
	require.NoError(t, err)
	require.Equal(t, string(before), string(after), "public projection must leave original rich parts, signatures and positions intact")
	c.Writer = chatWireInternalCapture{c.Writer}
	internal := ChatResponseForWire(c, &source)
	raw, err := json.Marshal(internal)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(raw), "internal capture must preserve the full rich source")
}

// TestGeminiPublicChatUnsupportedPartsVersion retains the strict default for an unsupported extension version.
func TestGeminiPublicChatUnsupportedPartsVersion(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			body, _ := renderChatWireFixture(t, `{"text":"one","thoughtSignature":"c2lnMQ=="},{"text":"two","thoughtSignature":"c2lnMg=="}`, stream, "v2", true, true)
			choices := strictChatWireChoices(t, body, stream)
			require.Len(t, choices, 1)
			message := choices[0].Message
			if stream {
				message = choices[0].Delta
			}
			require.NotNil(t, message.Content)
			require.Equal(t, "onetwo", *message.Content)
			require.NotContains(t, body, "gemini_part_index")
		})
	}
}

// TestGeminiChatPartsNegotiationHeaders preserves cache variation for both public representations.
func TestGeminiChatPartsNegotiationHeaders(t *testing.T) {
	for _, version := range []string{"", "v1", "v2"} {
		t.Run("version_"+version, func(t *testing.T) {
			c, recorder := signatureTestContext()
			c.Request.Header.Set(NativePartsHeader, version)
			c.Writer.Header().Add("Vary", "Origin")
			response := &openai.TextResponse{}
			ChatResponseForWire(c, response)
			ChatResponseForWire(c, response)
			vary := strings.Join(recorder.Header().Values("Vary"), ",")
			require.Contains(t, vary, "Origin")
			require.Equal(t, 1, strings.Count(vary, NativePartsHeader))
			expected := ""
			if version == "v1" {
				expected = "v1"
			}
			require.Equal(t, expected, recorder.Header().Get(NativePartsHeader))
		})
	}
}
