package adaptor

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
)

// liveRESTProbe records provider preparation without requiring real credentials.
// Its embedded interface supplies methods not used by DoRequestHelper.
type liveRESTProbe struct {
	Adaptor
	url               string
	urlCalls, headers int
}

// GetRequestURL records URL preparation and returns the local upstream URL.
// Parameters: m is request metadata. Returns: the fixture URL and no error.
func (p *liveRESTProbe) GetRequestURL(_ *meta.Meta) (string, error) {
	p.urlCalls++
	return p.url, nil
}

// SetupRequestHeader records credential preparation without adding credentials.
// Parameters: c, r, and m describe the request. Returns: no error.
func (p *liveRESTProbe) SetupRequestHeader(_ *gin.Context, _ *http.Request, _ *meta.Meta) error {
	p.headers++
	return nil
}

// GetChannelName returns a stable diagnostic name. Parameters: none.
// Returns: the fixture provider name.
func (p *liveRESTProbe) GetChannelName() string { return "live-rest-fixture" }

// liveRESTBodyProbe records whether the helper reads a request payload.
type liveRESTBodyProbe struct {
	io.Reader
	reads int
}

// Read records payload access and delegates to the fixture reader.
// Parameters: b receives bytes. Returns: the delegated byte count and read error.
func (p *liveRESTBodyProbe) Read(b []byte) (int, error) {
	p.reads++
	return p.Reader.Read(b)
}

// TestLiveOnlyModelsNeverReachREST verifies the shared dispatch boundary with a
// real local HTTP server, including streaming, mapped names, and positive controls.
// Parameters: t is the test handle. Returns: none.
func TestLiveOnlyModelsNeverReachREST(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(upstream.Close)
	previousClient, previousTraceMode := client.HTTPClient, config.TraceWriteMode
	client.HTTPClient, config.TraceWriteMode = upstream.Client(), "batch"
	t.Cleanup(func() {
		client.HTTPClient, config.TraceWriteMode = previousClient, previousTraceMode
	})

	for _, channel := range []int{channeltype.Gemini, channeltype.VertextAI, channeltype.GeminiOpenAICompatible, channeltype.OpenAICompatible} {
		for _, name := range []string{"gemini-3.8-live", "gemini-3.8-live-extended-thinking", "gemini-3.8-flash", "gemini-2.5-flash"} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("channel_%d/%s/stream_%t", channel, name, stream), func(t *testing.T) {
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
					gmw.SetLogger(c, logger.Logger)
					m := &meta.Meta{ChannelType: channel, OriginModelName: "friendly-alias", ActualModelName: name, IsStream: stream}
					provider := &liveRESTProbe{url: upstream.URL}
					body := &liveRESTBodyProbe{Reader: strings.NewReader(`{"messages":[]}`)}
					before := calls.Load()
					response, err := DoRequestHelper(provider, c, m, body)
					if response != nil {
						require.NoError(t, response.Body.Close())
					}
					t.Logf("url_calls=%d header_calls=%d body_reads=%d network_calls=%d error=%v", provider.urlCalls, provider.headers, body.reads, calls.Load()-before, err)
					liveOnly := name == "gemini-3.8-live" || name == "gemini-3.8-live-extended-thinking"
					if channel != channeltype.OpenAICompatible && liveOnly {
						require.ErrorContains(t, err, "Live API")
						require.Nil(t, response)
						require.Zero(t, provider.urlCalls, "reject before preparing an upstream URL")
						require.Zero(t, provider.headers, "reject before preparing credentials")
						require.Zero(t, body.reads, "reject before buffering the payload")
						require.Equal(t, before, calls.Load(), "no billable upstream attempt")
						require.False(t, c.GetBool(ctxkey.UpstreamRequestPossiblyForwarded))
						require.Empty(t, m.UpstreamRequestURL)
						return
					}
					// Other providers may implement their own REST bridge. Do not block
					// them by model name alone, or reject ordinary Gemini Flash models.
					require.NoError(t, err)
					require.NotNil(t, response)
					require.Equal(t, http.StatusNoContent, response.StatusCode)
					require.Equal(t, before+1, calls.Load())
					require.Equal(t, 1, provider.urlCalls)
					require.Equal(t, 1, provider.headers)
					require.Positive(t, body.reads)
					require.True(t, c.GetBool(ctxkey.UpstreamRequestPossiblyForwarded))
				})
			}
		}
	}
}
