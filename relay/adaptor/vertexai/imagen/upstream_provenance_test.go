package imagen

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// imagenProviderFailureStatuses lists the provider HTTP statuses that the
// adaptive ability cooldown must see as provider-caused failures.
var imagenProviderFailureStatuses = []int{
	http.StatusTooManyRequests,
	http.StatusInternalServerError,
	http.StatusServiceUnavailable,
}

// failingReadCloser is a response body whose Read always fails, standing in
// for a connection dropped while one-api reads an Imagen response.
type failingReadCloser struct{}

// Read always returns an error so the caller observes a local read failure.
// It accepts p and returns zero bytes with a wrapped error.
func (failingReadCloser) Read(p []byte) (int, error) {
	return 0, errors.New("simulated imagen body read failure")
}

// Close reports success because nothing is held open.
// It returns nil.
func (failingReadCloser) Close() error {
	return nil
}

// newImagenTestContext builds a gin test context with a request attached so
// adaptor code that derives a request context works as in the relay.
// It accepts t and returns the context.
func newImagenTestContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	return c
}

// fetchImagenProviderResponse starts a fake Vertex Imagen provider that
// answers with status and body, performs a real HTTP round trip to it, and
// returns the live response the relay would hand to DoResponse. It accepts t,
// status, and body.
func fetchImagenProviderResponse(t *testing.T, status int, body string) *http.Response {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		// t.Errorf is safe off the test goroutine; require would call FailNow here.
		if _, err := io.WriteString(w, body); err != nil {
			t.Errorf("write fake imagen provider body: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	resp, err := srv.Client().Post(srv.URL+"/v1/projects/p/locations/us-central1/publishers/google/models/imagen-4.0-generate-001:predict",
		"application/json", strings.NewReader(`{"instances":[{"prompt":"cat"}]}`))
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// requireUpstreamProvenance asserts that got is a provider-attributed error
// whose client-visible JSON equals what the legacy ErrorWrapper produced for
// the same message, code, and status. It accepts t, got, the expected message,
// code, and status.
func requireUpstreamProvenance(t *testing.T, got *model.ErrorWithStatusCode, message, code string, status int) {
	t.Helper()
	require.NotNil(t, got)
	require.Equal(t, status, got.StatusCode)
	require.True(t, got.FromUpstream, "provider status %d must be attributed to the provider", status)
	require.False(t, got.IsOneAPIError(), "provider status %d must not be treated as a one-api internal error", status)

	legacy := openai.ErrorWrapper(errors.New(message), code, status)
	gotJSON, err := json.Marshal(got)
	require.NoError(t, err)
	legacyJSON, err := json.Marshal(legacy)
	require.NoError(t, err)
	require.JSONEq(t, string(legacyJSON), string(gotJSON), "client-visible JSON must be unchanged")

	var wire map[string]any
	require.NoError(t, json.Unmarshal(gotJSON, &wire))
	require.Equal(t, message, wire["message"])
	require.Equal(t, string(model.ErrorTypeOneAPI), wire["type"])
	require.Equal(t, code, wire["code"])
	require.InDelta(t, float64(status), wire["status_code"], 0)
}

// requireLocalProvenance asserts that got is a failure one-api produced
// itself. It accepts t, got, the expected code, and the expected status.
func requireLocalProvenance(t *testing.T, got *model.ErrorWithStatusCode, code string, status int) {
	t.Helper()
	require.NotNil(t, got)
	require.Equal(t, status, got.StatusCode)
	require.Equal(t, code, got.Code)
	require.False(t, got.FromUpstream, "local failure %s must stay one-api owned", code)
	require.True(t, got.IsOneAPIError())
}

// TestDoResponse_ImageGenerationProviderStatusIsUpstream drives Adaptor.DoResponse
// in generation mode against a fake Imagen provider returning 429/500/503 and
// verifies the imagen_api_error is attributed to the provider and carries only
// the parsed provider message.
func TestDoResponse_ImageGenerationProviderStatusIsUpstream(t *testing.T) {
	t.Parallel()
	for _, status := range imagenProviderFailureStatuses {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			body := `{"error":{"code":` + strconv.Itoa(status) + `,"message":"imagen provider failure"}}`
			resp := fetchImagenProviderResponse(t, status, body)
			c := newImagenTestContext(t)

			usage, got := (&Adaptor{}).DoResponse(c, resp, &meta.Meta{Mode: relaymode.ImagesGenerations})
			require.Nil(t, usage)
			requireUpstreamProvenance(t, got, "imagen provider failure", "imagen_api_error", status)
		})
	}
}

// TestDoResponse_ImageEditProviderStatusIsUpstream drives Adaptor.DoResponse in
// edit mode, which delegates to HandleImageEdit, against a fake Imagen provider
// returning 429/500/503 and verifies provider attribution with the parsed
// provider message.
func TestDoResponse_ImageEditProviderStatusIsUpstream(t *testing.T) {
	t.Parallel()
	for _, status := range imagenProviderFailureStatuses {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			body := `{"error":{"status":"` + http.StatusText(status) + `","message":"imagen edit provider failure"}}`
			resp := fetchImagenProviderResponse(t, status, body)
			c := newImagenTestContext(t)

			usage, got := (&Adaptor{}).DoResponse(c, resp, &meta.Meta{Mode: relaymode.ImagesEdits})
			require.Nil(t, usage)
			requireUpstreamProvenance(t, got, "imagen edit provider failure", "imagen_api_error", status)
		})
	}
}

// TestHandleImageEdit_ProviderStatusIsUpstream calls the exported HandleImageEdit
// directly with live 429/500/503 provider responses and verifies attribution
// with a generic message, since a string error is not a Google envelope.
func TestHandleImageEdit_ProviderStatusIsUpstream(t *testing.T) {
	t.Parallel()
	for _, status := range imagenProviderFailureStatuses {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			body := `{"error":"edit quota exhausted"}`
			resp := fetchImagenProviderResponse(t, status, body)
			c := newImagenTestContext(t)

			usage, got := HandleImageEdit(c, resp)
			require.Nil(t, usage)
			requireUpstreamProvenance(t, got, "upstream returned HTTP "+strconv.Itoa(status), "imagen_api_error", status)
		})
	}
}

// TestImagenLocalFailuresStayOneAPIOwned is the control: failures one-api
// produces itself in the same functions (body read failure, undecodable 2xx
// body) must keep FromUpstream false.
func TestImagenLocalFailuresStayOneAPIOwned(t *testing.T) {
	t.Parallel()

	t.Run("DoResponse body read failure", func(t *testing.T) {
		t.Parallel()
		resp := &http.Response{StatusCode: http.StatusServiceUnavailable, Body: failingReadCloser{}}
		_, got := (&Adaptor{}).DoResponse(newImagenTestContext(t), resp, &meta.Meta{Mode: relaymode.ImagesGenerations})
		requireLocalProvenance(t, got, "read_response_body_failed", http.StatusInternalServerError)
	})

	t.Run("HandleImageEdit body read failure", func(t *testing.T) {
		t.Parallel()
		resp := &http.Response{StatusCode: http.StatusServiceUnavailable, Body: failingReadCloser{}}
		_, got := HandleImageEdit(newImagenTestContext(t), resp)
		requireLocalProvenance(t, got, "read_response_body_failed", http.StatusInternalServerError)
	})

	for _, mode := range []int{relaymode.ImagesGenerations, relaymode.ImagesEdits} {
		t.Run("undecodable 2xx body mode "+strconv.Itoa(mode), func(t *testing.T) {
			t.Parallel()
			resp := fetchImagenProviderResponse(t, http.StatusOK, "not-json")
			_, got := (&Adaptor{}).DoResponse(newImagenTestContext(t), resp, &meta.Meta{Mode: mode})
			requireLocalProvenance(t, got, "unmarshal_response_body_failed", http.StatusInternalServerError)
		})
	}
}
