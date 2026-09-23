package muapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

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
