package gemini

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/model"
)

// signatureTestContext returns an isolated HTTP request and response recorder for behavioral tests.
func signatureTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c, recorder
}

// TestGeminiThoughtSignatureInput preserves each opaque signature on its original function-call part.
func TestGeminiThoughtSignatureInput(t *testing.T) {
	var request model.GeneralOpenAIRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model":"gemini-3.8-flash","messages":[{"role":"assistant","tool_calls":[{"id":"a","type":"function","function":{"name":"lookup","arguments":"{}"},"extra_content":{"google":{"thought_signature":"c2lnLUE="}}},{"id":"b","type":"function","function":{"name":"lookup","arguments":"{}"}},{"id":"c","type":"function","function":{"name":"other","arguments":"{}"},"extra_content":{"google":{"thought_signature":"c2lnLUI="}}}]}]}`), &request))
	before, err := json.Marshal(request)
	require.NoError(t, err)
	converted := ConvertRequest(request)
	require.NotNil(t, converted)
	raw, err := json.Marshal(converted)
	require.NoError(t, err)
	var wire struct {
		Contents []struct {
			Parts []map[string]any `json:"parts"`
		} `json:"contents"`
	}
	require.NoError(t, json.Unmarshal(raw, &wire))
	require.Len(t, wire.Contents, 1)
	require.Len(t, wire.Contents[0].Parts, 3)
	require.Equal(t, "c2lnLUE=", wire.Contents[0].Parts[0]["thoughtSignature"])
	require.NotContains(t, wire.Contents[0].Parts[1], "thoughtSignature")
	require.Equal(t, "c2lnLUI=", wire.Contents[0].Parts[2]["thoughtSignature"])
	after, err := json.Marshal(request)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after))
}

// TestGeminiThoughtSignatureResponse exercises native response parsing and public Chat wire serialization.
func TestGeminiThoughtSignatureResponse(t *testing.T) {
	c, recorder := signatureTestContext()
	body := `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"lookup","args":{"q":"a"}},"thoughtSignature":"c2lnLUE="},{"functionCall":{"name":"other","args":{}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":7,"candidatesTokenCount":3,"totalTokenCount":10}}`
	failure, usage := Handler(c, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, 0, "gemini-3.8-flash")
	require.Nil(t, failure)
	require.Equal(t, 10, usage.TotalTokens)
	var wire struct {
		Choices []struct {
			Message struct {
				ToolCalls []map[string]any `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &wire))
	require.Len(t, wire.Choices, 1)
	calls := wire.Choices[0].Message.ToolCalls
	require.Len(t, calls, 2)
	require.Equal(t, map[string]any{"google": map[string]any{"thought_signature": "c2lnLUE="}, "oneapi": map[string]any{"gemini_part_index": float64(0)}}, calls[0]["extra_content"])
	require.Equal(t, map[string]any{"oneapi": map[string]any{"gemini_part_index": float64(1)}}, calls[1]["extra_content"])
	require.NotEqual(t, calls[0]["id"], calls[1]["id"])
	require.Equal(t, "tool_calls", wire.Choices[0].FinishReason)
}

// TestGeminiThoughtSignatureStream keeps distinct complete calls at contiguous per-candidate indexes across chunks.
func TestGeminiThoughtSignatureStream(t *testing.T) {
	c, recorder := signatureTestContext()
	body := "data: " + `{"candidates":[{"index":0,"content":{"parts":[{"text":"checking"},{"functionCall":{"name":"lookup","args":{"q":"a"}},"thoughtSignature":"c2lnLUE="}]}}]}` + "\n\n" +
		"data: " + `{"candidates":[{"index":0,"content":{"parts":[{"functionCall":{"name":"lookup","args":{"q":"b"}},"thoughtSignature":"c2lnLUI="}]}},{"index":1,"content":{"parts":[{"functionCall":{"name":"other","args":{}}}]}}]}` + "\n\n" +
		"data: " + `{"candidates":[{"index":0,"finishReason":"STOP"},{"index":1,"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":7,"candidatesTokenCount":3,"totalTokenCount":10}}` + "\n\n"
	failure, _, usage := StreamHandler(c, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))})
	require.Nil(t, failure)
	require.Equal(t, 10, usage.TotalTokens)
	calls := map[int][]map[string]any{}
	finishes := map[int]string{}
	for _, line := range strings.Split(recorder.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data: ") || strings.HasSuffix(line, "[DONE]") {
			continue
		}
		var chunk struct {
			Choices []struct {
				Index int `json:"index"`
				Delta struct {
					ToolCalls []map[string]any `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &chunk))
		for _, choice := range chunk.Choices {
			calls[choice.Index] = append(calls[choice.Index], choice.Delta.ToolCalls...)
			if choice.FinishReason != "" {
				finishes[choice.Index] = choice.FinishReason
			}
		}
	}
	require.Len(t, calls[0], 2)
	require.EqualValues(t, 0, calls[0][0]["index"])
	require.EqualValues(t, 1, calls[0][1]["index"])
	require.NotEqual(t, calls[0][0]["id"], calls[0][1]["id"])
	require.Equal(t, map[string]any{"google": map[string]any{"thought_signature": "c2lnLUE="}, "oneapi": map[string]any{"gemini_part_index": float64(1)}}, calls[0][0]["extra_content"])
	require.Equal(t, map[string]any{"google": map[string]any{"thought_signature": "c2lnLUI="}, "oneapi": map[string]any{"gemini_part_index": float64(2)}}, calls[0][1]["extra_content"])
	require.Len(t, calls[1], 1)
	require.EqualValues(t, 0, calls[1][0]["index"])
	require.NotContains(t, calls[1][0], "extra_content")
	require.Equal(t, "tool_calls", finishes[0])
	require.Equal(t, "tool_calls", finishes[1])
}

// TestGeminiThoughtSignatureTextParts preserves signed empty parts and thought markers without merging text.
func TestGeminiThoughtSignatureTextParts(t *testing.T) {
	c, recorder := signatureTestContext()
	body := `{"candidates":[{"content":{"role":"model","parts":[{"text":"first","thought":true,"thoughtSignature":"c2lnLUE="},{"text":"answer"},{"text":"","thoughtSignature":"c2lnLUI="}]},"finishReason":"STOP"}]}`
	failure, _ := Handler(c, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, 0, "gemini-3.8-flash")
	require.Nil(t, failure)
	var response struct {
		Choices []struct {
			Message model.Message `json:"message"`
		} `json:"choices"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	converted := ConvertRequest(model.GeneralOpenAIRequest{Model: "gemini-3.8-flash", Messages: []model.Message{response.Choices[0].Message}})
	require.Len(t, converted.Contents, 1)
	raw, err := json.Marshal(converted.Contents[0])
	require.NoError(t, err)
	require.JSONEq(t, `{"role":"model","parts":[{"text":"first","thought":true,"thoughtSignature":"c2lnLUE="},{"text":"answer"},{"text":"","thoughtSignature":"c2lnLUI="}]}`, string(raw))
	var scalar model.GeneralOpenAIRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model":"gemini-3.8-flash","messages":[{"role":"assistant","content":"signed scalar","extra_content":{"google":{"thought_signature":"c2lnLUM="}}}]}`), &scalar))
	scalarWire, err := json.Marshal(ConvertRequest(scalar).Contents[0])
	require.NoError(t, err)
	require.JSONEq(t, `{"role":"model","parts":[{"text":"signed scalar","thoughtSignature":"c2lnLUM="}]}`, string(scalarWire))
}

// TestGeminiThoughtSignatureEmptyTextStream preserves a signed empty final part through the Chat stream wire.
func TestGeminiThoughtSignatureEmptyTextStream(t *testing.T) {
	c, recorder := signatureTestContext()
	body := "data: " + `{"candidates":[{"index":0,"content":{"parts":[{"text":"","thoughtSignature":"c2lnLWZpbmFs"}]},"finishReason":"STOP"}]}` + "\n\n"
	failure, _, _ := StreamHandler(c, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))})
	require.Nil(t, failure)
	require.Contains(t, recorder.Body.String(), `"thought_signature":"c2lnLWZpbmFs"`)
	require.Contains(t, recorder.Body.String(), `"text":""`)
}

// TestGeminiThoughtSignatureRejectsUnsupportedFragments prevents unrequested incremental calls from being silently erased.
func TestGeminiThoughtSignatureRejectsUnsupportedFragments(t *testing.T) {
	for _, field := range []string{`"partialArgs":[{"jsonPath":"$.q","stringValue":"a"}]`, `"willContinue":true`} {
		for _, stream := range []bool{false, true} {
			c, recorder := signatureTestContext()
			payload := `{"candidates":[{"content":{"parts":[{"functionCall":{"name":"lookup",` + field + `}}]}}],"usageMetadata":{"promptTokenCount":7,"candidatesTokenCount":3,"totalTokenCount":10}}`
			response := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(payload))}
			if stream {
				response.Body = io.NopCloser(strings.NewReader("data: " + payload + "\n\n"))
				failure, _, usage := StreamHandler(c, response)
				require.NotNil(t, failure)
				require.NotNil(t, usage)
				require.Equal(t, 10, usage.TotalTokens)
			} else {
				failure, usage := Handler(c, response, 0, "gemini-3.8-flash")
				require.NotNil(t, failure)
				require.NotNil(t, usage)
				require.Equal(t, 10, usage.TotalTokens)
			}
			require.NotContains(t, recorder.Body.String(), `"tool_calls"`)
		}
	}
}

// TestGeminiThoughtSignatureFinishControls retains empty nonterminal filtering and normalized terminal reasons.
func TestGeminiThoughtSignatureFinishControls(t *testing.T) {
	c, _ := signatureTestContext()
	require.Nil(t, streamResponseGeminiChat2OpenAI(c, &ChatResponse{Candidates: []ChatCandidate{{Index: 0}}}))
	for native, want := range map[string]string{"STOP": "stop", "MAX_TOKENS": "length", "SAFETY": "content_filter"} {
		response := responseGeminiChat2OpenAI(c, &ChatResponse{Candidates: []ChatCandidate{{FinishReason: native, Content: ChatContent{Parts: []Part{{Text: "answer"}}}}}})
		require.Equal(t, want, response.Choices[0].FinishReason)
	}
}

// TestGeminiThoughtSignatureFragmentNamesInArguments accepts business data that resembles protocol metadata.
func TestGeminiThoughtSignatureFragmentNamesInArguments(t *testing.T) {
	c, recorder := signatureTestContext()
	payload := `{"candidates":[{"content":{"parts":[{"functionCall":{"name":"lookup","args":{"partialArgs":[1],"willContinue":true}}}]},"finishReason":"STOP"}]}`
	failure, _ := Handler(c, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(payload))}, 0, "gemini-3.8-flash")
	require.Nil(t, failure)
	var response struct {
		Choices []struct {
			Message model.Message `json:"message"`
		} `json:"choices"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	arguments, err := response.Choices[0].Message.ToolCalls[0].Function.ArgumentsJSON()
	require.NoError(t, err)
	require.JSONEq(t, `{"partialArgs":[1],"willContinue":true}`, arguments)
}

// TestGeminiThoughtSignatureCompleteDefaults accepts protobuf defaults and normalizes absent Struct arguments.
func TestGeminiThoughtSignatureCompleteDefaults(t *testing.T) {
	for _, fields := range []string{``, `,"partialArgs":[],"willContinue":false`, `,"partialArgs":null`} {
		c, recorder := signatureTestContext()
		payload := `{"candidates":[{"content":{"parts":[{"functionCall":{"name":"lookup"` + fields + `}}]},"finishReason":"STOP"}]}`
		failure, _ := Handler(c, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(payload))}, 0, "gemini-3.8-flash")
		require.Nil(t, failure)
		var response struct {
			Choices []struct {
				Message model.Message `json:"message"`
			} `json:"choices"`
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
		arguments, err := response.Choices[0].Message.ToolCalls[0].Function.ArgumentsJSON()
		require.NoError(t, err)
		require.JSONEq(t, `{}`, arguments)
	}
}

// TestGeminiThoughtSignatureMixedOrder replays gateway-positioned text and function calls in original native order.
func TestGeminiThoughtSignatureMixedOrder(t *testing.T) {
	c, recorder := signatureTestContext()
	payload := `{"candidates":[{"content":{"parts":[{"text":"before"},{"functionCall":{"name":"lookup","args":{}},"thoughtSignature":"c2lnLUE="},{"text":"between","thoughtSignature":"c2lnLUI="},{"functionCall":{"name":"other","args":{}}},{"text":"","thoughtSignature":"c2lnLUM="}]},"finishReason":"STOP"}]}`
	failure, _ := Handler(c, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(payload))}, 0, "gemini-3.8-flash")
	require.Nil(t, failure)
	var response struct {
		Choices []struct {
			Message model.Message `json:"message"`
		} `json:"choices"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	converted, err := ConvertNativeRequestChecked(c, model.GeneralOpenAIRequest{Model: "gemini-3.8-flash", Messages: []model.Message{response.Choices[0].Message}})
	require.NoError(t, err)
	actual, err := json.Marshal(converted.Contents[0])
	require.NoError(t, err)
	require.JSONEq(t, `{"role":"model","parts":[{"text":"before"},{"functionCall":{"name":"lookup","args":{}},"thoughtSignature":"c2lnLUE="},{"text":"between","thoughtSignature":"c2lnLUI="},{"functionCall":{"name":"other","args":{}}},{"text":"","thoughtSignature":"c2lnLUM="}]}`, string(actual))
}

// TestGeminiThoughtSignatureInvalidPositions rejects incomplete or invalid gateway ordering rather than guessing.
func TestGeminiThoughtSignatureInvalidPositions(t *testing.T) {
	for _, indices := range []string{`0,0`, `1,2`, `-1,0`, `0,null`} {
		values := strings.Split(indices, ",")
		raw := `{"model":"gemini-3.8-flash","messages":[{"role":"assistant","content":[{"type":"text","text":"a","extra_content":{"oneapi":{"gemini_part_index":` + values[0] + `}}},{"type":"text","text":"b","extra_content":{"oneapi":{"gemini_part_index":` + values[1] + `}}}]}]}`
		var request model.GeneralOpenAIRequest
		require.NoError(t, json.Unmarshal([]byte(raw), &request))
		c, _ := signatureTestContext()
		_, err := ConvertNativeRequestChecked(c, request)
		require.Error(t, err)
	}
}

// TestGeminiThoughtSignaturePlainStreamControl preserves ordinary scalar text without a tool-capable request.
func TestGeminiThoughtSignaturePlainStreamControl(t *testing.T) {
	c, recorder := signatureTestContext()
	payload := "data: " + `{"candidates":[{"content":{"parts":[{"text":"plain"}]},"finishReason":"STOP"}]}` + "\n\n"
	failure, _, _ := StreamHandler(c, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(payload))})
	require.Nil(t, failure)
	require.Contains(t, recorder.Body.String(), `"content":"plain"`)
	require.NotContains(t, recorder.Body.String(), `gemini_part_index`)
}

// TestGeminiThoughtSignatureCompleteDefaultsStream retains argumentless complete calls and protobuf defaults in SSE.
func TestGeminiThoughtSignatureCompleteDefaultsStream(t *testing.T) {
	for _, fields := range []string{``, `,"partialArgs":[],"willContinue":false`, `,"partialArgs":null`} {
		c, recorder := signatureTestContext()
		payload := "data: " + `{"candidates":[{"index":0,"content":{"parts":[{"functionCall":{"name":"lookup"` + fields + `}}]},"finishReason":"STOP"}]}` + "\n\n"
		failure, _, _ := StreamHandler(c, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(payload))})
		require.Nil(t, failure)
		line := strings.Split(recorder.Body.String(), "\n")[0]
		var response struct {
			Choices []struct {
				Delta model.Message `json:"delta"`
			} `json:"choices"`
		}
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &response))
		arguments, err := response.Choices[0].Delta.ToolCalls[0].Function.ArgumentsJSON()
		require.NoError(t, err)
		require.JSONEq(t, `{}`, arguments)
	}
}

// TestGeminiThoughtSignatureToolCapableStreamOrder preserves an early unsigned text frame before a later function call.
func TestGeminiThoughtSignatureToolCapableStreamOrder(t *testing.T) {
	c, recorder := signatureTestContext()
	EnableVertexPartPositions(c)
	c.Set(ctxkey.ConvertedRequest, &ChatRequest{Tools: []ChatTools{{}}})
	body := "data: " + `{"candidates":[{"index":0,"content":{"parts":[{"text":"before"}]}}]}` + "\n\n" +
		"data: " + `{"candidates":[{"index":0,"content":{"parts":[{"functionCall":{"name":"lookup","args":{}},"thoughtSignature":"c2lnLUE="}]}}]}` + "\n\n" +
		"data: " + `{"candidates":[{"index":0,"content":{"parts":[{"text":"after"}]},"finishReason":"STOP"}]}` + "\n\n"
	failure, _, _ := StreamHandler(c, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))})
	require.Nil(t, failure)
	message := model.Message{Role: "assistant"}
	var content []model.MessageContent
	for _, line := range strings.Split(recorder.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data: ") || strings.HasSuffix(line, "[DONE]") {
			continue
		}
		var event struct {
			Choices []struct {
				Delta model.Message `json:"delta"`
			} `json:"choices"`
		}
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event))
		for _, choice := range event.Choices {
			message.ToolCalls = append(message.ToolCalls, choice.Delta.ToolCalls...)
			parts, err := choice.Delta.ParseContentChecked()
			require.NoError(t, err)
			content = append(content, parts...)
		}
	}
	message.Content = content
	converted, err := ConvertNativeRequestChecked(c, model.GeneralOpenAIRequest{Model: "gemini-3.8-flash", Messages: []model.Message{message}})
	require.NoError(t, err)
	raw, err := json.Marshal(converted.Contents[0])
	require.NoError(t, err)
	require.JSONEq(t, `{"role":"model","parts":[{"text":"before"},{"functionCall":{"name":"lookup","args":{}},"thoughtSignature":"c2lnLUE="},{"text":"after"}]}`, string(raw))
}

// TestGeminiThoughtSignatureLateNoToolContext reports incomplete positional replay instead of fabricating earlier boundaries.
func TestGeminiThoughtSignatureLateNoToolContext(t *testing.T) {
	c, recorder := signatureTestContext()
	body := "data: " + `{"candidates":[{"index":0,"content":{"parts":[{"text":"before"}]}}]}` + "\n\n" +
		"data: " + `{"candidates":[{"index":0,"content":{"parts":[{"text":"","thoughtSignature":"c2lnLUE="}]},"finishReason":"STOP"}]}` + "\n\n"
	failure, _, _ := StreamHandler(c, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))})
	require.Nil(t, failure)
	require.Contains(t, recorder.Body.String(), `"content":"before"`)
	var content []model.MessageContent
	for _, line := range strings.Split(recorder.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data: ") || strings.HasSuffix(line, "[DONE]") {
			continue
		}
		var event struct {
			Choices []struct {
				Delta model.Message `json:"delta"`
			} `json:"choices"`
		}
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event))
		for _, choice := range event.Choices {
			parts, err := choice.Delta.ParseContentChecked()
			require.NoError(t, err)
			content = append(content, parts...)
		}
	}
	_, err := ConvertNativeRequestChecked(c, model.GeneralOpenAIRequest{Model: "gemini-3.8-flash", Messages: []model.Message{{Role: "assistant", Content: content}}})
	require.ErrorContains(t, err, "positions must cover every part")
}
