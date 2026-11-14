package vertexai

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/gemini"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestVertexContinuationMockRoundTrip exercises an offline transport and actual response adapter.
// Its historical shared-converter control reproduces the missing-native-result rejection.
func TestVertexContinuationMockRoundTrip(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var wire struct {
			Contents []struct {
				Role  string `json:"role"`
				Parts []struct {
					Call   *gemini.FunctionCall     `json:"functionCall"`
					Result *gemini.FunctionResponse `json:"functionResponse"`
				} `json:"parts"`
			} `json:"contents"`
		}
		if err := json.NewDecoder(r.Body).Decode(&wire); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		var calls, results []string
		for _, content := range wire.Contents {
			for _, part := range content.Parts {
				if part.Call != nil {
					calls = append(calls, part.Call.FunctionName)
				}
				if part.Result != nil {
					if content.Role != "user" {
						http.Error(w, "invalid result role", 400)
						return
					}
					results = append(results, part.Result.Name)
				}
			}
		}
		if len(calls) != len(results) {
			http.Error(w, "missing native functionResponse", 400)
			return
		}
		for i := range calls {
			if calls[i] != results[i] {
				http.Error(w, "result order mismatch", 400)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, err := io.WriteString(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"Both cities checked."}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":3,"totalTokenCount":13}}`)
		if err != nil {
			return
		}
	}))
	defer server.Close()
	request := vertexContinuationRequest()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	historical, err := gemini.ConvertNativeRequestChecked(c, *request)
	require.NoError(t, err)
	historicalRaw, err := json.Marshal(historical)
	require.NoError(t, err)
	response, err := server.Client().Post(server.URL, "application/json", bytes.NewReader(historicalRaw))
	require.NoError(t, err)
	historicalBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, 400, response.StatusCode)
	require.Contains(t, string(historicalBody), "missing native functionResponse")
	// Plain chat proves the transport does not require function results indiscriminately.
	plainRaw := vertexSchemaWire(t, vertexSchemaRequest(nil))
	response, err = server.Client().Post(server.URL, "application/json", bytes.NewReader(plainRaw))
	require.NoError(t, err)
	require.Equal(t, 200, response.StatusCode)
	require.NoError(t, response.Body.Close())
	response, err = server.Client().Post(server.URL, "application/json", bytes.NewReader(vertexSchemaWire(t, request)))
	require.NoError(t, err)
	require.Equal(t, 200, response.StatusCode)
	recorder := httptest.NewRecorder()
	c, _ = gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	usage, apiErr := (&Adaptor{}).DoResponse(c, response, &meta.Meta{Mode: relaymode.ChatCompletions, ActualModelName: request.Model})
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Equal(t, 13, usage.TotalTokens)
	require.Contains(t, recorder.Body.String(), "Both cities checked.")
}

// TestVertexContinuationCompletedBatches allows IDs reused across distinct completed batches.
func TestVertexContinuationCompletedBatches(t *testing.T) {
	request := vertexContinuationRequest()
	request.Messages = append(request.Messages, request.Messages[1:]...)
	var wire struct {
		Contents []struct {
			Role string `json:"role"`
		} `json:"contents"`
	}
	require.NoError(t, json.Unmarshal(vertexSchemaWire(t, request), &wire))
	require.Len(t, wire.Contents, 5)
}

// TestVertexContinuationSystemAlignment checks native system instructions and legacy dummy turns.
func TestVertexContinuationSystemAlignment(t *testing.T) {
	for _, modelName := range []string{"gemini-3.8-flash", "gemini-1.0-pro"} {
		t.Run(modelName, func(t *testing.T) {
			request := vertexContinuationRequest()
			request.Model = modelName
			request.Messages = append([]model.Message{{Role: "system", Content: "Stay concise."}}, request.Messages...)
			request.Messages[2].Content = "Checking now."
			raw := vertexSchemaWire(t, request)
			require.Contains(t, string(raw), "Stay concise.")
			require.Contains(t, string(raw), "Checking now.")
			require.Contains(t, string(raw), "functionResponse")
		})
	}
}
