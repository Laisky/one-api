package adaptor

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/relay/meta"
)

// TestReviewEndpointCredentialDispatch exercises real HTTP dispatch and captured
// diagnostics for unrecognized credential keys and malformed query separators.
func TestReviewEndpointCredentialDispatch(t *testing.T) {
	oldSinks := config.TraceSinks
	config.TraceSinks = []string{config.TraceSinkNone}
	t.Cleanup(func() { config.TraceSinks = oldSinks })
	previous := client.HTTPClient
	client.HTTPClient = &http.Client{}
	t.Cleanup(func() { client.HTTPClient = previous })
	for _, query := range []string{"credential=fixture-secret", "token=fixture-secret;broken=1", "custom=fixture-secret&safe=1"} {
		for _, override := range []bool{false, true} {
			runURLDiagnosticScenario(t, urlDiagnosticScenario{query: query, secrets: []string{"fixture-secret"}},
				urlDiagnosticResponse{name: "ok", contentType: "application/json", body: `{}`, status: http.StatusOK}, override)
		}
	}
}

// TestReviewTransportErrorURLRedaction uses a canceled real client request so
// URL redaction cannot hide the error cause or change the outbound request.
func TestReviewTransportErrorURLRedaction(t *testing.T) {
	previous := client.HTTPClient
	client.HTTPClient = &http.Client{}
	t.Cleanup(func() { client.HTTPClient = previous })
	raw := "https://user:fixture-secret@example.com/v1/chat?credential=fixture-secret#fixture-secret"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, raw, nil)
	require.NoError(t, err)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	_, err = DoRequest(c, req)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	require.NotContains(t, err.Error(), "fixture-secret")
	var urlErr *url.Error
	require.True(t, errors.As(err, &urlErr))
	require.Equal(t, "https://example.com/v1/chat", urlErr.URL)
	require.Equal(t, raw, req.URL.String())
	// Request construction is a separate failure path before Client.Do.
	a := &stubAdaptor{defaultURL: "https://user:fixture-secret@example.com/%zz?credential=fixture-secret"}
	_, err = DoRequestHelper(a, c, &meta.Meta{}, strings.NewReader("{}"))
	require.Error(t, err)
	require.NotContains(t, err.Error(), "fixture-secret")
}
