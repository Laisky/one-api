package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestReviewChannelProbeURLPrivacy exercises the actual operator probe, including
// URL override dispatch and both initial/base-URL and prepared-URL diagnostics.
func TestReviewChannelProbeURLPrivacy(t *testing.T) {
	setupChannelSweepTestEnvironment(t)
	const secret = "fixture-probe-url-secret"
	seen := make(chan map[string]string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- map[string]string{"query": r.URL.RawQuery, "path": r.URL.Path, "auth": r.Header.Get("Authorization")}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"probe","object":"chat.completion","model":"gpt-4o-mini","choices":[{"message":{"role":"assistant","content":"PROBE_OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":1,"total_tokens":8}}`))
	}))
	defer upstream.Close()
	oldClient, oldSinks := client.HTTPClient, config.TraceSinks
	client.HTTPClient, config.TraceSinks = upstream.Client(), []string{config.TraceSinkNone}
	defer func() { client.HTTPClient, config.TraceSinks = oldClient, oldSinks }()
	base, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	base.User = url.UserPassword("operator", secret)
	rawBase := base.String()
	override := rawBase + "/v1/chat/completions?credential=" + secret
	cfg, err := json.Marshal(model.ChannelConfig{EndpointURLs: map[string]string{"chat_completions": override}})
	require.NoError(t, err)
	channel := &model.Channel{Name: "private-probe", Type: channeltype.OpenAICompatible, Status: model.ChannelStatusEnabled, Key: "probe-fixture-key", BaseURL: &rawBase, Models: "gpt-4o-mini", Config: string(cfg)}
	require.NoError(t, model.DB.Create(channel).Error)
	core, observed := observer.New(zapcore.DebugLevel)
	lg, err := glog.NewWithName("probe-url-review", glog.LevelDebug, zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
	require.NoError(t, err)
	response, probeErr, apiErr := testChannel(gmw.SetLogger(context.Background(), lg), channel, buildTestRequest("gpt-4o-mini"))
	// Wait for the actual probe's asynchronous audit write before restoring its DB.
	require.Eventually(t, func() bool {
		var n int64
		return model.DB.Model(&model.Log{}).Where("channel_id = ?", channel.Id).Count(&n).Error == nil && n == 1
	}, 5*time.Second, 10*time.Millisecond)
	require.NoError(t, probeErr)
	require.Nil(t, apiErr)
	require.Contains(t, response, "PROBE_OK")
	got := <-seen
	require.Equal(t, "credential="+secret, got["query"], "do not sanitize the real dispatch")
	require.Equal(t, "/v1/chat/completions", got["path"])
	require.Equal(t, "Bearer probe-fixture-key", got["auth"])
	prepared := observed.FilterMessage("prepare test request").All()
	require.Len(t, prepared, 1)
	require.Equal(t, upstream.URL+"/v1/chat/completions", prepared[0].ContextMap()["upstream_url"])
	initial := observed.FilterMessage("channel test: initial model context").All()
	require.Len(t, initial, 1)
	require.Equal(t, upstream.URL, initial[0].ContextMap()["base_url"])
	for _, entry := range observed.All() {
		require.NotContains(t, entry.Message, secret)
		for _, field := range entry.Context {
			require.NotContains(t, field.String, secret)
		}
	}
	require.Equal(t, rawBase, *channel.BaseURL)
}
