package xai

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestVideoGatewayURL verifies the gateway creation route maps to xAI's native
// endpoint. Parameters: t runs table assertions. Returns: none; no API is called.
func TestVideoGatewayURL(t *testing.T) {
	for _, path := range []string{"/v1/videos", "/v1/videos/generations"} {
		got, err := (&Adaptor{}).GetRequestURL(&metalib.Meta{
			Mode: relaymode.Videos, BaseURL: "https://api.x.ai", RequestURLPath: path,
			ChannelType: channeltype.XAI,
		})
		require.NoError(t, err)
		require.Equal(t, "https://api.x.ai/v1/videos/generations", got)
	}
}

// TestVideoGatewaySubmission exercises the real adaptor HTTP boundary and task
// response. Parameters: t controls the fake upstream. Returns: none; no live key
// or billable provider is used and zero identities disable task persistence.
func TestVideoGatewaySubmission(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v1/videos/generations", r.URL.Path)
		require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.JSONEq(t, `{"model":"grok-imagine-video-1.5","prompt":"test","duration":6,"resolution":"720p"}`, string(body))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"request_id":"video-test-id"}`)
	}))
	defer upstream.Close()

	ctx, recorder := gin.CreateTestContext(httptest.NewRecorder())
	_ = recorder
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	ctx.Request.Header.Set("Content-Type", "application/json")
	meta := &metalib.Meta{Mode: relaymode.Videos, BaseURL: upstream.URL,
		RequestURLPath: "/v1/videos", ChannelType: channeltype.XAI,
		APIKey: "test-key", ActualModelName: "grok-imagine-video-1.5"}
	metalib.Set2Context(ctx, meta)
	ad := &Adaptor{}
	resp, err := ad.DoRequest(ctx, meta, strings.NewReader(`{"model":"grok-imagine-video-1.5","prompt":"test","duration":6,"resolution":"720p"}`))
	require.NoError(t, err)
	_, relayErr := ad.DoResponse(ctx, resp, meta)
	require.Nil(t, relayErr)
}

// TestVideoGatewayTaskResponse ensures accepted tasks expose both the native
// request_id and the gateway binding id. Parameters: t supplies a recorder.
// Returns: none; the handler cannot contact a provider or persist zero identities.
func TestVideoGatewayTaskResponse(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	meta := &metalib.Meta{Mode: relaymode.Videos, ChannelType: channeltype.XAI}
	metalib.Set2Context(ctx, meta)
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header),
		Body: io.NopCloser(strings.NewReader(`{"request_id":"video-test-id"}`))}
	_, relayErr := (&Adaptor{}).DoResponse(ctx, resp, meta)
	require.Nil(t, relayErr)
	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.JSONEq(t, `"video-test-id"`, string(body["id"]))
	require.JSONEq(t, `"video-test-id"`, string(body["request_id"]))
}
