package adaptor

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// diagnosticRequest captures an upstream request for assertions on the test goroutine.
type diagnosticRequest struct {
	method, path, rawQuery, body, authorization, contentType, accept string
	readErr                                                          error
}

// runURLDiagnosticScenario sends one real HTTP request using the test-owned
// scenario, response and override flag. It returns no value and checks both
// the actual dispatch and every diagnostic URL field, including duplicates.
func runURLDiagnosticScenario(t *testing.T, tc urlDiagnosticScenario, response urlDiagnosticResponse, override bool) {
	t.Helper()
	const headerSecret = "fixture-header-secret"
	const unusedSecret = "fixture-unused-adaptor-secret"
	received := make(chan diagnosticRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		received <- diagnosticRequest{
			method: r.Method, path: r.URL.Path, rawQuery: r.URL.RawQuery,
			body: string(body), readErr: err,
			authorization: r.Header.Get("Authorization"),
			contentType:   r.Header.Get("Content-Type"), accept: r.Header.Get("Accept"),
		}
		w.Header().Set("Content-Type", response.contentType)
		w.Header().Set("X-Upstream-Marker", "preserved")
		w.WriteHeader(response.status)
		if _, err := io.WriteString(w, response.body); err != nil {
			t.Errorf("write upstream fixture: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	requestURL, logURL := server.URL+"/v1/rerank", server.URL+"/v1/rerank"
	if tc.query != "" {
		requestURL += "?" + tc.query
		logURL += "?" + tc.wantQuery
	}
	core, observed := observer.New(zapcore.DebugLevel)
	lg, err := glog.NewWithName("url-diagnostics-test", glog.LevelDebug,
		zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
	require.NoError(t, err)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/rerank", nil)
	c.Request = c.Request.WithContext(gmw.SetLogger(c, lg))
	c.Set(ctxkey.ContentType, "application/json")
	m := &meta.Meta{
		Mode: relaymode.Rerank, ChannelId: 42, ActualModelName: "fixture-model",
		APIKey: headerSecret,
		Config: model.ChannelConfig{CustomHeaders: map[string]string{
			"Authorization": "Bearer {{key}}", "Accept": response.contentType,
		}},
	}
	a := &stubAdaptor{defaultURL: requestURL}
	if override {
		a.defaultURL = "http://unused-adaptor.invalid/unused?key=" + unusedSecret
		m.Config.EndpointURLs = map[string]string{"rerank": requestURL}
	}
	var body io.Reader
	if !tc.nilBody {
		body = strings.NewReader(tc.body)
		if tc.unknownSize {
			body = struct{ io.Reader }{body}
		}
	}
	resp, err := DoRequestHelper(a, c, m, body)
	require.NoError(t, err)
	require.NotNil(t, resp)
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, response.status, resp.StatusCode)
	require.Equal(t, response.body, string(responseBody))
	require.Equal(t, response.contentType, resp.Header.Get("Content-Type"))
	require.Equal(t, "preserved", resp.Header.Get("X-Upstream-Marker"))
	require.Equal(t, requestURL, m.UpstreamRequestURL)
	require.True(t, c.GetBool(ctxkey.UpstreamRequestPossiblyForwarded))
	got := <-received
	require.NoError(t, got.readErr)
	require.Equal(t, http.MethodPost, got.method)
	require.Equal(t, "/v1/rerank", got.path)
	require.Equal(t, tc.query, got.rawQuery, "never sanitize the actual upstream request")
	require.Equal(t, tc.body, got.body)
	require.Equal(t, "Bearer "+headerSecret, got.authorization)
	require.Equal(t, "application/json", got.contentType)
	require.Equal(t, response.contentType, got.accept)

	forwarded := observed.FilterMessage("forwarding request to upstream channel").All()
	require.Len(t, forwarded, 1, "keep useful diagnostics, rather than hiding the leak by deleting logs")
	errorLogs := observed.FilterMessage("upstream returned error status").All()
	if response.status >= http.StatusBadRequest {
		require.Len(t, errorLogs, 1)
		require.EqualValues(t, response.status, errorLogs[0].ContextMap()["status"])
	} else {
		require.Empty(t, errorLogs)
	}
	for _, entry := range append(forwarded, errorLogs...) {
		// ContextMap would silently discard duplicate keys. Inspect
		// every field, including the URL inherited from lg.With.
		urlFields := 0
		for _, field := range entry.Context {
			if field.Key == "url" {
				urlFields++
				require.Equal(t, logURL, field.String, entry.Message)
			}
		}
		require.Positive(t, urlFields)
		require.Equal(t, "stub", entry.ContextMap()["adaptor"])
		require.Equal(t, "fixture-model", entry.ContextMap()["model"])
	}
	fields := forwarded[0].ContextMap()
	require.Equal(t, http.MethodPost, fields["method"])
	require.Equal(t, true, fields["body_logging_suppressed"])
	_, hasSize := fields["body_bytes"]
	require.Equal(t, !tc.nilBody && !tc.unknownSize, hasSize)
	if hasSize {
		require.EqualValues(t, len(tc.body), fields["body_bytes"])
	}
	secrets := append([]string{headerSecret, unusedSecret, "fixture-private-prompt", "fixture-private-document"}, tc.secrets...)
	for _, entry := range observed.All() {
		for _, secret := range secrets {
			require.NotContains(t, entry.Message, secret)
			for _, field := range entry.Context {
				require.NotContains(t, field.String, secret)
			}
		}
	}
}
