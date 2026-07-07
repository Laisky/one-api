package adaptor

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// withTraceDB points model.DB at a throwaway in-memory database for the duration
// of the test so DoRequestHelper's best-effort trace bookkeeping does not panic
// on a nil DB. It restores the previous handle on cleanup.
func withTraceDB(t *testing.T) {
	t.Helper()
	prev := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Trace{}))
	model.DB = db
	t.Cleanup(func() { model.DB = prev })
}

// stubAdaptor is a minimal Adaptor implementation whose GetRequestURL returns a
// deliberately unreachable default URL. It lets the dispatch test prove that a
// per-endpoint override actually replaces the adaptor-computed URL.
type stubAdaptor struct {
	defaultURL string
}

func (s *stubAdaptor) Init(*meta.Meta) {}

func (s *stubAdaptor) GetRequestURL(*meta.Meta) (string, error) { return s.defaultURL, nil }

func (s *stubAdaptor) SetupRequestHeader(*gin.Context, *http.Request, *meta.Meta) error { return nil }

func (s *stubAdaptor) ConvertRequest(*gin.Context, int, *relaymodel.GeneralOpenAIRequest) (any, error) {
	return nil, nil
}

func (s *stubAdaptor) ConvertImageRequest(*gin.Context, *relaymodel.ImageRequest) (any, error) {
	return nil, nil
}

func (s *stubAdaptor) ConvertClaudeRequest(*gin.Context, *relaymodel.ClaudeRequest) (any, error) {
	return nil, nil
}

func (s *stubAdaptor) DoRequest(*gin.Context, *meta.Meta, io.Reader) (*http.Response, error) {
	return nil, nil
}

func (s *stubAdaptor) DoResponse(*gin.Context, *http.Response, *meta.Meta) (*relaymodel.Usage, *relaymodel.ErrorWithStatusCode) {
	return nil, nil
}

func (s *stubAdaptor) GetModelList() []string { return nil }

func (s *stubAdaptor) GetChannelName() string { return "stub" }

func (s *stubAdaptor) GetDefaultModelPricing() map[string]ModelConfig { return nil }

func (s *stubAdaptor) GetModelRatio(string) float64 { return 1 }

func (s *stubAdaptor) GetCompletionRatio(string) float64 { return 1 }

// TestDoRequestHelperAppliesEndpointOverride verifies that DoRequestHelper sends
// the upstream request to the administrator-configured per-endpoint URL override
// instead of the adaptor-computed default, and records it on meta.
func TestDoRequestHelperAppliesEndpointOverride(t *testing.T) {
	withTraceDB(t)

	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	overrideURL := server.URL + "/custom/rerank"

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "http://client.example/v1/rerank", strings.NewReader(`{}`))

	m := &meta.Meta{
		Mode: relaymode.Rerank,
		Config: model.ChannelConfig{
			EndpointURLs: map[string]string{
				"rerank": overrideURL,
			},
		},
	}

	// GetRequestURL points at an unreachable host; if the override is ignored the
	// request would fail against it instead of reaching the test server.
	a := &stubAdaptor{defaultURL: "http://default-should-not-be-used.invalid/v1/rerank"}

	resp, err := DoRequestHelper(a, c, m, strings.NewReader(`{}`))
	require.NoError(t, err)
	require.NotNil(t, resp)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "/custom/rerank", gotPath)
	require.Equal(t, overrideURL, m.UpstreamRequestURL)
}

// TestDoRequestHelperNoOverrideUsesDefault verifies that when no override is set,
// DoRequestHelper falls back to the adaptor-computed URL.
func TestDoRequestHelperNoOverrideUsesDefault(t *testing.T) {
	withTraceDB(t)

	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "http://client.example/v1/rerank", strings.NewReader(`{}`))

	m := &meta.Meta{Mode: relaymode.Rerank}
	a := &stubAdaptor{defaultURL: server.URL + "/v1/rerank"}

	resp, err := DoRequestHelper(a, c, m, strings.NewReader(`{}`))
	require.NoError(t, err)
	require.NotNil(t, resp)
	defer resp.Body.Close()

	require.Equal(t, "/v1/rerank", gotPath)
	require.Equal(t, server.URL+"/v1/rerank", m.UpstreamRequestURL)
}
