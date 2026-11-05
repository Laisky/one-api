package gemini

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/meta"
)

type review607CountTransport struct{ base string }

func (p review607CountTransport) GetRequestURL(*meta.Meta) (string, error) {
	return p.base + "/v1/models/mapped:generateContent", nil
}
func (p review607CountTransport) SetupRequestHeader(_ *gin.Context, r *http.Request, _ *meta.Meta) error {
	r.Header.Set("Authorization", "Bearer count-fixture")
	return nil
}

// TestReview607CountWire verifies provider schema, auth and the prepared media
// across Developer/Vertex counting without contacting a real provider.
func TestReview607CountWire(t *testing.T) {
	for _, provider := range []int{apitype.Gemini, apitype.VertexAI} {
		t.Run(map[int]string{apitype.Gemini: "gemini", apitype.VertexAI: "vertex"}[provider], func(t *testing.T) {
			type observed struct {
				path, auth string
				body       map[string]any
				err        error
			}
			seen := make(chan observed, 1)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var payload map[string]any
				err := json.NewDecoder(r.Body).Decode(&payload)
				seen <- observed{r.URL.Path, r.Header.Get("Authorization"), payload, err}
				_, _ = io.WriteString(w, `{"totalTokens":102,"promptTokensDetails":[{"modality":"TEXT","tokenCount":2},{"modality":"VIDEO","tokenCount":100}]}`)
			}))
			defer server.Close()
			old := client.HTTPClient
			client.HTTPClient = server.Client()
			defer func() { client.HTTPClient = old }()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/", nil)
			request := &ChatRequest{Contents: []ChatContent{{Role: "user", Parts: []Part{{Text: "before"}, {InlineData: &InlineData{MimeType: "video/mp4", Data: "YQ=="}}}}}, SystemInstruction: &ChatContent{Parts: []Part{{Text: "system"}}}}
			before, err := json.Marshal(request)
			require.NoError(t, err)
			usage, err := CountNativeChatTokens(c, &meta.Meta{APIType: provider, ActualModelName: "gemini-3.1-flash-image"}, request, review607CountTransport{server.URL})
			require.NoError(t, err)
			require.Equal(t, 102, usage.PromptTokens)
			got := <-seen
			require.NoError(t, got.err)
			require.Equal(t, "/v1/models/mapped:countTokens", got.path)
			require.Equal(t, "Bearer count-fixture", got.auth)
			payload := got.body
			if provider == apitype.Gemini {
				payload = got.body["generateContentRequest"].(map[string]any)
				require.Equal(t, "models/gemini-3.1-flash-image", payload["model"])
			} else {
				require.NotContains(t, payload, "generateContentRequest")
			}
			require.Contains(t, payload, "systemInstruction")
			require.NotContains(t, payload, "generationConfig")
			require.NotContains(t, payload, "generation_config")
			encoded, err := json.Marshal(payload["contents"])
			require.NoError(t, err)
			require.Contains(t, string(encoded), `"mimeType":"video/mp4"`)
			after, err := json.Marshal(request)
			require.NoError(t, err)
			require.Equal(t, before, after, "counting must not modify the provider-visible generation")
		})
	}
}

// TestReview607CountFailClosed prevents dispatch after a missing or invalid
// token receipt, and makes sure upstream error bodies cannot leak media URLs.
func TestReview607CountFailClosed(t *testing.T) {
	for _, tc := range []struct {
		name, body       string
		status           int
		audio, cancelled bool
	}{
		{"zero", `{"totalTokens":0}`, 200, false, false},
		{"negative", `{"totalTokens":-1}`, 200, false, false},
		{"error", `{"error":{"message":"https://private.invalid/secret"}}`, 403, false, false},
		{"malformed", `not JSON`, 200, false, false},
		{"detail-overflow", `{"totalTokens":1,"promptTokensDetails":[{"modality":"VIDEO","tokenCount":2}]}`, 200, false, false},
		{"missing-audio", `{"totalTokens":100}`, 200, true, false},
		{"cancelled", `{"totalTokens":100}`, 200, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			old := client.HTTPClient
			client.HTTPClient = server.Client()
			defer func() { client.HTTPClient = old }()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/", nil)
			if tc.cancelled {
				ctx, cancel := context.WithCancel(c.Request.Context())
				cancel()
				c.Request = c.Request.WithContext(ctx)
			}
			mimeType := "video/mp4"
			if tc.audio {
				mimeType = "audio/wav"
			}
			request := &ChatRequest{Contents: []ChatContent{{Parts: []Part{{InlineData: &InlineData{MimeType: mimeType, Data: "YQ=="}}}}}}
			usage, err := CountNativeChatTokens(c, &meta.Meta{APIType: apitype.Gemini, ActualModelName: "gemini-2.5-flash"}, request, review607CountTransport{server.URL})
			require.Error(t, err)
			require.Nil(t, usage)
			require.False(t, strings.Contains(err.Error(), "private.invalid"))
		})
	}
}
