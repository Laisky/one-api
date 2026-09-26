package muapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGetRequestURLMapsMuAPIVideoLifecycle verifies model submission and task
// polling URL construction without contacting the provider.
func TestGetRequestURLMapsMuAPIVideoLifecycle(t *testing.T) {
	t.Parallel()
	a := &Adaptor{}
	cases := []struct {
		name string
		meta *meta.Meta
		want string
	}{
		{
			name: "creation",
			meta: &meta.Meta{
				Mode:            relaymode.Videos,
				BaseURL:         "https://api.muapi.ai",
				ActualModelName: "veo3-fast",
				RequestURLPath:  "/v1/videos/generations",
			},
			want: "https://api.muapi.ai/api/v1/veo3-fast",
		},
		{
			name: "creation with openai base suffix",
			meta: &meta.Meta{
				Mode:            relaymode.Videos,
				BaseURL:         "https://api.muapi.ai/v1",
				ActualModelName: "kling-v2.6-pro",
				RequestURLPath:  "/v1/videos",
			},
			want: "https://api.muapi.ai/api/v1/kling-v2.6-pro",
		},
		{
			name: "polling",
			meta: &meta.Meta{
				Mode:           relaymode.Videos,
				BaseURL:        "https://api.muapi.ai",
				RequestURLPath: "/v1/videos/job-abc_123",
			},
			want: "https://api.muapi.ai/api/v1/predictions/job-abc_123/result",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := a.GetRequestURL(tc.meta)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestGetRequestURLRejectsUnsafeModel ensures an untrusted model name cannot
// escape the fixed MuAPI endpoint path.
func TestGetRequestURLRejectsUnsafeModel(t *testing.T) {
	t.Parallel()
	_, err := (&Adaptor{}).GetRequestURL(&meta.Meta{
		Mode:            relaymode.Videos,
		BaseURL:         "https://api.muapi.ai",
		ActualModelName: "../internal",
		RequestURLPath:  "/v1/videos",
	})
	require.Error(t, err)
}

// TestGetRequestURLAcceptsUnlistedCatalogSlug proves newly published MuAPI models do not require a code change.
func TestGetRequestURLAcceptsUnlistedCatalogSlug(t *testing.T) {
	t.Parallel()

	url, err := (&Adaptor{}).GetRequestURL(&meta.Meta{
		Mode:            relaymode.Videos,
		BaseURL:         "https://api.muapi.ai",
		ActualModelName: "new-video-model-v3",
		RequestURLPath:  "/v1/videos",
	})
	require.NoError(t, err)
	require.Equal(t, "https://api.muapi.ai/api/v1/new-video-model-v3", url)
}

// TestSetupRequestHeaderUsesMuAPIKey verifies that native video requests use
// MuAPI's documented x-api-key header rather than bearer authentication.
func TestSetupRequestHeaderUsesMuAPIKey(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	c := newMuAPITestContext(http.MethodPost, "/v1/videos", `{"model":"veo3-fast","duration":5}`)
	req := httptest.NewRequest(http.MethodPost, "https://api.muapi.ai/api/v1/veo3-fast", nil)
	metaInfo := &meta.Meta{APIKey: "test-key", IsStream: false}

	require.NoError(t, (&Adaptor{}).SetupRequestHeader(c, req, metaInfo))
	require.Equal(t, "test-key", req.Header.Get("x-api-key"))
	require.Empty(t, req.Header.Get("Authorization"))
}

// TestVideoCreationRedirectsNeverForwardCredentialsOrPaidPayload verifies the
// adaptor refuses POST redirects before following them, including redirects
// whose HTTP semantics would replay the generation body.
func TestVideoCreationRedirectsNeverForwardCredentialsOrPaidPayload(t *testing.T) {
	for _, status := range []int{http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			previousTraceSinks := config.TraceSinks
			config.TraceSinks = []string{config.TraceSinkNone}
			t.Cleanup(func() { config.TraceSinks = previousTraceSinks })
			var targetRequests atomic.Int32
			var badAuth atomic.Bool
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				targetRequests.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
			}))
			defer target.Close()

			source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("x-api-key") != "secret-key" {
					badAuth.Store(true)
				}
				http.Redirect(w, r, target.URL+"/collect", status)
			}))
			defer source.Close()

			previousClient := client.HTTPClient
			client.HTTPClient = source.Client()
			t.Cleanup(func() { client.HTTPClient = previousClient })

			c := newMuAPITestContext(http.MethodPost, "/v1/videos", `{"duration":5,"prompt":"a lighthouse"}`)
			c.Set(ctxkey.ContentType, "application/json")
			response, err := (&Adaptor{}).DoRequest(c, &meta.Meta{
				Mode: relaymode.Videos, BaseURL: source.URL, APIKey: "secret-key",
				ActualModelName: "veo3-fast", RequestURLPath: "/v1/videos", ChannelId: 1,
			}, strings.NewReader(`{"duration":5,"prompt":"a lighthouse"}`))
			require.Error(t, err)
			require.Nil(t, response)
			require.False(t, badAuth.Load())
			require.Zero(t, targetRequests.Load(), "redirect target must receive neither x-api-key nor generation body")
		})
	}
}
