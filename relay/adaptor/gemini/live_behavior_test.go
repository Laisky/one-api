package gemini

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
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

const liveFixtureReceipt = `{"usageMetadata":{"promptTokenCount":200,"responseTokenCount":110,"totalTokenCount":310,"thoughtsTokenCount":3,"promptTokensDetails":[{"modality":"TEXT","tokenCount":100},{"modality":"AUDIO","tokenCount":100}],"responseTokensDetails":[{"modality":"TEXT","tokenCount":10},{"modality":"AUDIO","tokenCount":100}]}}`

// liveFixtureResult transfers completed handler state across the test boundary.
type liveFixtureResult struct {
	usage    *model.Usage
	biz      *model.ErrorWithStatusCode
	endpoint string
}

// liveFixture creates a real upstream and downstream WebSocket pair. Parameters:
// t, channel and name configure the session; serve runs after upstream upgrade.
// Returns: the gateway URL and joined handler result. No paid provider is called.
func liveFixture(t *testing.T, channel int, name string, serve func(*websocket.Conn) error) (string, <-chan liveFixtureResult) {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Goog-Api-Key") != "provider-fixture" || r.Header.Get("Authorization") != "" || r.URL.RawQuery != "" {
			t.Error("credentials were not isolated")
		}
		if !strings.HasSuffix(r.URL.Path, ".GenerativeService.BidiGenerateContent") {
			t.Error("wrong Gemini endpoint path")
		}
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
			t.Error(err)
			return
		}
		if err := serve(conn); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(upstream.Close)
	results := make(chan liveFixtureResult, 1)
	engine := gin.New()
	engine.GET("/v1/realtime", func(c *gin.Context) {
		gmw.SetLogger(c, logger.Logger)
		m := &meta.Meta{ChannelType: channel, Mode: relaymode.Realtime, ActualModelName: name, OriginModelName: "friendly", BaseURL: upstream.URL, APIKey: "provider-fixture"}
		biz, usage := LiveHandler(c, m)
		if biz != nil {
			c.JSON(biz.StatusCode, gin.H{"error": biz.Error})
		}
		results <- liveFixtureResult{usage, biz, m.UpstreamRequestURL}
	})
	gateway := httptest.NewServer(engine)
	t.Cleanup(gateway.Close)
	return "ws" + strings.TrimPrefix(gateway.URL, "http") + "/v1/realtime?model=friendly", results
}

// acknowledgeLiveFixture checks the rewritten setup before acknowledging it.
// Parameters: conn is the provider socket and name is its bound model.
// Returns: an error if setup is invalid or cannot be acknowledged.
func acknowledgeLiveFixture(conn *websocket.Conn, name string) error {
	var setup struct {
		Setup struct {
			Model string `json:"model"`
		} `json:"setup"`
	}
	if err := conn.ReadJSON(&setup); err != nil {
		return fmt.Errorf("read fixture setup: %w", err)
	}
	if setup.Setup.Model != "models/"+name {
		return fmt.Errorf("setup was not model-pinned")
	}
	return liveWrite(conn, websocket.TextMessage, []byte(`{"setupComplete":{}}`), time.Second)
}

// connectLiveFixture opens the gateway and completes setup. Parameters: t and
// endpoint identify the fixture. Returns: a socket with a bounded read deadline.
func connectLiveFixture(t *testing.T, endpoint string) *websocket.Conn {
	t.Helper()
	conn, response, err := websocket.DefaultDialer.Dial(endpoint, http.Header{"Authorization": []string{"Bearer downstream-fixture"}})
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(8*time.Second)))
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"setup":{"model":"friendly","inputAudioTranscription":{},"outputAudioTranscription":{},"tools":[{"functionDeclarations":[{"name":"lookup","parameters":{"type":"OBJECT"}}]}]}}`)))
	_, ack, err := conn.ReadMessage()
	require.NoError(t, err)
	require.JSONEq(t, `{"setupComplete":{}}`, string(ack))
	return conn
}

// receiveLiveFixture waits for the handler and both pump readers to finish.
// Parameters: t and results select the session. Returns: its settled usage.
func receiveLiveFixture(t *testing.T, results <-chan liveFixtureResult) *model.Usage {
	t.Helper()
	select {
	case result := <-results:
		require.Nil(t, result.biz)
		require.NotNil(t, result.usage)
		require.NotContains(t, result.endpoint, "provider-fixture")
		return result.usage
	case <-time.After(8 * time.Second):
		t.Fatal("Live handler did not join its pumps")
		return nil
	}
}

// TestGeminiLiveBidirectionalFramesAndReceipts tests both channel types and Live
// models through actual sockets. Parameters: t is a test. Returns: none. Audio,
// transcripts, cancellation and function results retain their native wire data.
func TestGeminiLiveBidirectionalFramesAndReceipts(t *testing.T) {
	for _, channel := range []int{channeltype.Gemini, channeltype.GeminiOpenAICompatible} {
		for _, name := range []string{"gemini-3.8-live", "gemini-3.8-live-extended-thinking"} {
			t.Run(fmt.Sprintf("%d/%s", channel, name), func(t *testing.T) {
				input := `{"realtimeInput":{"text":"Hello","audio":{"mimeType":"audio/pcm;rate=16000","data":"AQI="}}}`
				toolResponse := `{"toolResponse":{"functionResponses":[{"id":"call-1","name":"lookup","response":{"id":9007199254740993}}]}}`
				frames := []string{
					`{"serverContent":{"modelTurn":{"parts":[{"inlineData":{"mimeType":"audio/pcm;rate=24000","data":"AQI="}}]}}}`,
					`{"serverContent":{"inputTranscription":{"text":"Hello","finished":true}}}`,
					`{"serverContent":{"outputTranscription":{"text":"Hello back","finished":true}}}`,
					`{"toolCall":{"functionCalls":[{"id":"call-1","name":"lookup","args":{"id":9007199254740993}}]}}`,
					`{"toolCallCancellation":{"ids":["obsolete-call"]}}`,
					liveFixtureReceipt, liveFixtureReceipt,
					`{"serverContent":{"turnComplete":true,"interactionStatus":"IDLE"}}`,
				}
				endpoint, results := liveFixture(t, channel, name, func(up *websocket.Conn) error {
					if err := acknowledgeLiveFixture(up, name); err != nil {
						return err
					}
					_, raw, err := up.ReadMessage()
					if err != nil {
						return err
					}
					if string(raw) != input {
						return fmt.Errorf("client frame changed")
					}
					for i, frame := range frames {
						kind := websocket.TextMessage
						if i == 0 {
							kind = websocket.BinaryMessage
						}
						if err := liveWrite(up, kind, []byte(frame), time.Second); err != nil {
							return err
						}
						if i == 3 {
							_, raw, err := up.ReadMessage()
							if err != nil {
								return err
							}
							if string(raw) != toolResponse {
								return fmt.Errorf("function response changed")
							}
						}
					}
					liveClose(up, websocket.CloseNormalClosure, "fixture_complete")
					return nil
				})
				client := connectLiveFixture(t, endpoint)
				require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(input)))
				for i, want := range frames {
					kind, raw, err := client.ReadMessage()
					require.NoError(t, err)
					require.Equal(t, want, string(raw))
					if i == 0 {
						require.Equal(t, websocket.BinaryMessage, kind)
					}
					if i == 3 {
						require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(toolResponse)))
					}
				}
				require.NoError(t, client.Close())
				usage := receiveLiveFixture(t, results)
				require.Len(t, usage.Realtime.Records, 1)
				require.False(t, usage.Realtime.HasUsageGap())
				require.Equal(t, 200, usage.PromptTokens)
				require.Equal(t, 110, usage.CompletionTokens)
			})
		}
	}
}

// TestGeminiLiveDrainsLateReceipt verifies paid usage survives a downstream
// disconnect. Parameters: t is a test. Returns: none; barriers replace sleeps.
func TestGeminiLiveDrainsLateReceipt(t *testing.T) {
	inputSeen, release := make(chan struct{}), make(chan struct{})
	endpoint, results := liveFixture(t, channeltype.Gemini, "gemini-3.8-live", func(up *websocket.Conn) error {
		if err := acknowledgeLiveFixture(up, "gemini-3.8-live"); err != nil {
			return err
		}
		if _, _, err := up.ReadMessage(); err != nil {
			return err
		}
		close(inputSeen)
		select {
		case <-release:
		case <-time.After(5 * time.Second):
			return fmt.Errorf("late receipt barrier timeout")
		}
		raw := strings.TrimSuffix(liveFixtureReceipt, "}") + `,"serverContent":{"modelTurn":{},"turnComplete":true}}`
		return liveWrite(up, websocket.TextMessage, []byte(raw), time.Second)
	})
	client := connectLiveFixture(t, endpoint)
	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"realtimeInput":{"text":"hi"}}`)))
	select {
	case <-inputSeen:
	case <-time.After(5 * time.Second):
		t.Fatal("input not forwarded")
	}
	require.NoError(t, client.Close())
	close(release)
	usage := receiveLiveFixture(t, results)
	require.Len(t, usage.Realtime.Records, 1)
	require.Equal(t, 110, usage.CompletionTokens)
	require.False(t, usage.Realtime.HasUsageGap())
}

// TestGeminiLiveRejectsForgedUsageAndRepeatedSetup verifies that client frames
// cannot mint usage or switch models. Parameters: t is a test. Returns: none.
func TestGeminiLiveRejectsForgedUsageAndRepeatedSetup(t *testing.T) {
	for _, frame := range []string{`{"usageMetadata":{"totalTokenCount":0}}`, `{"setup":{"model":"another-model"}}`, `{"toolResponse":{"functionResponses":[{"id":"unsolicited","response":{}}]}}`} {
		t.Run(frame, func(t *testing.T) {
			endpoint, results := liveFixture(t, channeltype.Gemini, "gemini-3.8-live", func(up *websocket.Conn) error {
				if err := acknowledgeLiveFixture(up, "gemini-3.8-live"); err != nil {
					return err
				}
				if _, raw, err := up.ReadMessage(); err == nil {
					return fmt.Errorf("forged client operation reached provider: %d bytes", len(raw))
				}
				return nil
			})
			client := connectLiveFixture(t, endpoint)
			require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(frame)))
			_, _, err := client.ReadMessage()
			require.True(t, websocket.IsCloseError(err, websocket.ClosePolicyViolation))
			usage := receiveLiveFixture(t, results)
			require.Empty(t, usage.Realtime.Records)
			require.False(t, usage.Realtime.HasUsageGap())
		})
	}
}

// TestGeminiLiveSetupAndURLPolicy checks precise admission boundaries.
// Parameters: t is a test. Returns: none; positive controls remain accepted.
func TestGeminiLiveSetupAndURLPolicy(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{`{"setup":{"model":"other"}}`, `{"setup":{"model":"friendly","model":"other"}}`, `{"setup":{"tools":[{"googleSearch":{}}]}}`, `{"setup":{"sessionResumption":{"handle":"untrusted"}}}`, `{"setup":{"generationConfig":{"responseModalities":["TEXT"]}}}`, `{"setup":{"generationConfig":{"thinkingConfig":{"thinkingLevel":"HIGH"}}}}`, `{"setup":{"generationConfig":{"thinking_config":{}}}}`, strings.Repeat("[", 65) + strings.Repeat("]", 65)} {
		_, err := prepareLiveSetup([]byte(raw), "gemini-3.8-live", "friendly")
		require.Error(t, err)
	}
	for _, level := range []string{"low", "medium", "high"} {
		raw, err := prepareLiveSetup([]byte(`{"setup":{"generationConfig":{"thinkingConfig":{"thinkingLevel":"`+level+`"}}}}`), "gemini-3.8-live-extended-thinking", "friendly")
		require.NoError(t, err)
		require.Contains(t, string(raw), strings.ToUpper(level))
	}
	m := &meta.Meta{ChannelType: channeltype.Gemini, Mode: relaymode.Realtime, ActualModelName: "gemini-3.8-live"}
	for _, base := range []string{"https://generativelanguage.googleapis.com", "https://generativelanguage.googleapis.com/v1beta/openai", "ws://127.0.0.1:1234"} {
		m.BaseURL = base
		endpoint, err := LiveRequestURL(m)
		require.NoError(t, err)
		require.Contains(t, endpoint, "v1alpha.GenerativeService.BidiGenerateContent")
	}
	for _, base := range []string{"http://example.com", "https://user:secret@example.com", "https://example.com?key=secret", "https://example.com#secret"} {
		m.BaseURL = base
		_, err := LiveRequestURL(m)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "secret")
	}
}

// TestGeminiLiveToolStateIsConnectionBound verifies native function identities
// cannot leak across sessions. Parameters: t is a test. Returns: none.
func TestGeminiLiveToolStateIsConnectionBound(t *testing.T) {
	t.Parallel()
	first, second := &liveToolState{}, &liveToolState{}
	require.NoError(t, first.observe([]byte(`{"toolCall":{"functionCalls":[{"id":"a","name":"lookup"}]}}`)))
	response := []byte(`{"toolResponse":{"functionResponses":[{"id":"a","name":"lookup","response":{}}]}}`)
	_, err := second.accept(response)
	require.Error(t, err)
	isTool, err := first.accept(response)
	require.NoError(t, err)
	require.True(t, isTool)
	_, err = first.accept(response)
	require.Error(t, err)
	require.NoError(t, first.observe([]byte(`{"toolCall":{"functionCalls":[{"id":"b","name":"lookup"}]}}`)))
	require.NoError(t, first.observe([]byte(`{"toolCallCancellation":{"ids":["b"]}}`)))
	raw := strings.Replace(string(response), `"a"`, `"b"`, 1)
	_, err = first.accept([]byte(raw))
	require.Error(t, err)
	// Tool schemas and results remain arbitrary JSON, including large integers.
	var object map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(response, &object))
}
