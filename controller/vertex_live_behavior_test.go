package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/relay/adaptor/vertexai"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// vertexLiveFixtureMeta supplies operator-owned configuration without accessing
// any Google account. Parameters: name is the configured model. Returns: metadata.
func vertexLiveFixtureMeta(name string) *meta.Meta {
	m := &meta.Meta{ChannelType: channeltype.VertextAI, Mode: relaymode.Realtime,
		ActualModelName: name, OriginModelName: "my-live-alias", ChannelId: 984211}
	m.Config.VertexAIProjectID = "operator-project"
	m.Config.Region = "europe-west4"
	m.Config.VertexAIADC = `{"type":"service_account","client_email":"fixture@example.invalid"}`
	return m
}

// TestVertexLivePublishedModelsRemainConfigurable reproduces catalog filtering
// without credentials or entitlement probes. Parameters: t owns the test. Returns: none.
func TestVertexLivePublishedModelsRemainConfigurable(t *testing.T) {
	t.Parallel()
	models := (&vertexai.Adaptor{}).GetModelList()
	for _, name := range []string{"gemini-3.8-live", "gemini-3.8-live-extended-thinking",
		"gemini-3.1-flash-live-preview", "gemini-3.5-live-translate-preview", "gemini-3.5-transcribe-live"} {
		t.Run(name, func(t *testing.T) { require.Contains(t, models, name) })
	}
	require.Contains(t, models, "gemini-3.8-flash")
}

// TestVertexLiveEndpointMetadata advertises the implemented transport through
// the same handler used by channel configuration. Parameters: t owns the test. Returns: none.
func TestVertexLiveEndpointMetadata(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/channel/metadata?type=%d", channeltype.VertextAI), nil)
	GetChannelMetadata(c)
	var body struct {
		Success bool `json:"success"`
		Data struct { Endpoints []string `json:"default_endpoints"` } `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.Contains(t, body.Data.Endpoints, "realtime")
}

// TestVertexLiveEligibilityDoesNotGuessEntitlements accepts operator-configured
// models without consulting IAM, release stage, or a model allowlist. Parameters:
// t owns the test. Returns: none; the upstream remains the authority on access.
func TestVertexLiveEligibilityDoesNotGuessEntitlements(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"gemini-3.8-live", "gemini-3.8-live-extended-thinking", "operator-configured-live-id"} {
		t.Run(name, func(t *testing.T) {
			m := vertexLiveFixtureMeta(name)
			require.True(t, isGeminiLiveRequest(m))
			require.NoError(t, validateGeminiRealtimeTransport(m))
			require.Equal(t, "vertexai", resolveRealtimePricingAdaptor(m).GetChannelName())
		})
	}
}

// TestVertexLiveNativeDispatchAndReceipt exercises actual WebSockets through
// controller dispatch, OAuth isolation, resource binding, and the receipt ledger.
// Parameters: t owns the fixtures. Returns: none; no paid provider is contacted.
func TestVertexLiveNativeDispatchAndReceipt(t *testing.T) {
	m := vertexLiveFixtureMeta("gemini-3.8-live")
	vertexai.Cache.Set("vertexai-token-984211", "vertex-fixture-token", time.Minute)
	t.Cleanup(func() { vertexai.Cache.Delete("vertexai-token-984211") })
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer vertex-fixture-token" || r.Header.Get("X-Goog-Api-Key") != "" || r.URL.RawQuery != "" {
			t.Error("Vertex credentials were not isolated")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/ws/google.cloud.aiplatform.v1.LlmBidiService/BidiGenerateContent" {
			t.Errorf("wrong Vertex endpoint: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		u := websocket.Upgrader{}
		conn, err := u.Upgrade(w, r, nil)
		if err != nil { t.Error(err); return }
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(5*time.Second))
		var setup struct { Setup struct { Model string `json:"model"` } `json:"setup"` }
		if err := conn.ReadJSON(&setup); err != nil { t.Error(err); return }
		if setup.Setup.Model != "projects/operator-project/locations/europe-west4/publishers/google/models/gemini-3.8-live" {
			t.Errorf("setup did not bind the configured Vertex resource: %s", setup.Setup.Model)
			return
		}
		if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"setupComplete":{}}`)); err != nil { t.Error(err); return }
		_, input, err := conn.ReadMessage()
		if err != nil { t.Error(err); return }
		if string(input) != `{"realtimeInput":{"text":"Hello"}}` { t.Error("native input changed"); return }
		frame := `{"serverContent":{"modelTurn":{"parts":[{"inlineData":{"mimeType":"audio/pcm;rate=24000","data":"AQI="}}]},"turnComplete":true},"usageMetadata":{"promptTokenCount":10,"responseTokenCount":20,"totalTokenCount":30,"promptTokensDetails":[{"modality":"TEXT","tokenCount":10}],"responseTokensDetails":[{"modality":"AUDIO","tokenCount":20}]}}`
		if err := conn.WriteMessage(websocket.TextMessage, []byte(frame)); err != nil { t.Error(err); return }
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "complete"), time.Now().Add(time.Second))
	}))
	t.Cleanup(upstream.Close)
	m.BaseURL = upstream.URL
	results := make(chan *relaymodel.Usage, 1)
	engine := gin.New()
	engine.GET("/v1/realtime", func(c *gin.Context) {
		gmw.SetLogger(c, logger.Logger)
		if err := validateGeminiRealtimeTransport(m); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()}); return
		}
		biz, usage := runRealtimeProviderWithGemini(c, m)
		if biz != nil { c.JSON(biz.StatusCode, gin.H{"error": biz.Error}); return }
		results <- usage
	})
	gateway := httptest.NewServer(engine)
	t.Cleanup(gateway.Close)
	client, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(gateway.URL,"http")+"/v1/realtime?model=my-live-alias",
		http.Header{"Authorization": []string{"Bearer caller-fixture"}})
	if resp != nil && resp.Body != nil { defer resp.Body.Close() }
	require.NoError(t, err)
	defer client.Close()
	require.NoError(t, client.SetReadDeadline(time.Now().Add(5*time.Second)))
	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"setup":{"model":"my-live-alias"}}`)))
	_, ack, err := client.ReadMessage()
	require.NoError(t, err)
	require.JSONEq(t, `{"setupComplete":{}}`, string(ack))
	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"realtimeInput":{"text":"Hello"}}`)))
	_, frame, err := client.ReadMessage()
	require.NoError(t, err)
	require.Contains(t, string(frame), "usageMetadata")
	select {
	case usage := <-results:
		require.NotNil(t, usage)
		require.NotNil(t, usage.Realtime)
		require.False(t, usage.Realtime.HasUsageGap())
		require.Len(t, usage.Realtime.Records, 1)
		require.Equal(t, 10, usage.PromptTokens)
		require.Equal(t, 20, usage.CompletionTokens)
	case <-time.After(5*time.Second):
		t.Fatal("Vertex session did not finish with usage")
	}
}
