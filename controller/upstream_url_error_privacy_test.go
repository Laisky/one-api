package controller

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/identity"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// urlPrivacyFailureTransport implements a test-owned failure without external I/O.
type urlPrivacyFailureTransport func(*http.Request) (*http.Response, error)

// RoundTrip passes req to the fixture and returns its response and original cause.
func (f urlPrivacyFailureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// urlPrivacyFailureAdaptor overrides only the methods used by DoRequestHelper.
type urlPrivacyFailureAdaptor struct {
	adaptor.Adaptor
	requestURL string
}

// GetRequestURL returns the fixture URL without changing the supplied metadata.
func (a *urlPrivacyFailureAdaptor) GetRequestURL(*meta.Meta) (string, error) {
	return a.requestURL, nil
}

// GetChannelName returns the stable fixture provider name without side effects.
func (*urlPrivacyFailureAdaptor) GetChannelName() string { return "url-privacy-fixture" }

// SetupRequestHeader leaves headers to the real channel-config overlay and returns nil.
func (*urlPrivacyFailureAdaptor) SetupRequestHeader(*gin.Context, *http.Request, *meta.Meta) error {
	return nil
}

// TestUpstreamTransportFailureURLPrivacy exercises real http.Client wrapping,
// ErrorWrapper, the final Gin response, and Zap error serialization. It verifies
// diagnostics are sanitized while dispatch, error types and causes stay intact.
func TestUpstreamTransportFailureURLPrivacy(t *testing.T) {
	oldClient, oldSinks := client.HTTPClient, config.TraceSinks
	config.TraceSinks = []string{config.TraceSinkNone}
	t.Cleanup(func() { client.HTTPClient, config.TraceSinks = oldClient, oldSinks })

	const secret = "fixture-transport-query-secret"
	const headerSecret = "fixture-transport-header-secret"
	const body = `{"query":"fixture-private-transport-body"}`
	for _, path := range []string{"direct", "adaptor", "override"} {
		for _, query := range []struct{ name, raw, safe string }{
			{"credentials", "api-version=v1&key=" + secret + "&token=" + secret,
				"api-version=v1&key=%5Bredacted%5D&token=%5Bredacted%5D"},
			{"control", "token_count=2&api-version=v1", "token_count=2&api-version=v1"},
		} {
			for _, failure := range []struct {
				name    string
				cause   error
				timeout bool
			}{
				{"sentinel", errors.New("fixture dial failure"), false},
				{"cancelled", context.Canceled, false},
				{"deadline", context.DeadlineExceeded, true},
				{"dns", &net.DNSError{Err: "fixture DNS timeout", Name: "upstream.invalid", IsTimeout: true, IsTemporary: true}, true},
			} {
				t.Run(path+"/"+query.name+"/"+failure.name, func(t *testing.T) {
					requestURL := "https://upstream.invalid/v1/rerank?" + query.raw
					wantURL := "https://upstream.invalid/v1/rerank?" + query.safe
					var logs bytes.Buffer
					core := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(&logs), zapcore.DebugLevel)
					lg, err := glog.NewWithName("transport-url-privacy", glog.LevelDebug,
						zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
					require.NoError(t, err)
					recorder := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(recorder)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/rerank", nil)
					c.Request = c.Request.WithContext(gmw.SetLogger(c, lg))
					c.Set(ctxkey.ContentType, "application/json")

					var captured *http.Request
					var receivedBody []byte
					var readErr, closeErr error
					client.HTTPClient = &http.Client{Transport: urlPrivacyFailureTransport(func(req *http.Request) (*http.Response, error) {
						captured = req
						receivedBody, readErr = io.ReadAll(req.Body)
						closeErr = req.Body.Close()
						return nil, failure.cause
					})}
					var resp *http.Response
					if path == "direct" {
						req, makeErr := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, requestURL, strings.NewReader(body))
						require.NoError(t, makeErr)
						req.Header.Set("Authorization", "Bearer "+headerSecret)
						resp, err = adaptor.DoRequest(c, req)
					} else {
						m := &meta.Meta{Mode: relaymode.Rerank, ChannelId: 42, ActualModelName: "fixture-model", APIKey: headerSecret,
							Config: dbmodel.ChannelConfig{CustomHeaders: map[string]string{"Authorization": "Bearer {{key}}"}}}
						a := &urlPrivacyFailureAdaptor{requestURL: requestURL}
						if path == "override" {
							a.requestURL = "https://unused.invalid/rerank?key=fixture-unused-key"
							m.Config.EndpointURLs = map[string]string{"rerank": requestURL}
						}
						resp, err = adaptor.DoRequestHelper(a, c, m, strings.NewReader(body))
						require.Equal(t, requestURL, m.UpstreamRequestURL)
						require.True(t, c.GetBool(ctxkey.UpstreamRequestPossiblyForwarded))
						require.NotEmpty(t, identity.Fields(err), "channel identity must survive error sanitization")
					}
					require.Error(t, err)
					require.Nil(t, resp)
					require.NotNil(t, captured)
					require.NoError(t, readErr)
					require.NoError(t, closeErr)
					require.Equal(t, requestURL, captured.URL.String(), "never modify the outbound request URL")
					require.Equal(t, body, string(receivedBody))
					require.Equal(t, "Bearer "+headerSecret, captured.Header.Get("Authorization"))
					require.ErrorIs(t, err, failure.cause)
					var urlErr *url.Error
					require.ErrorAs(t, err, &urlErr)
					require.Equal(t, "Post", urlErr.Op)
					require.Equal(t, failure.cause, urlErr.Err)
					require.Equal(t, failure.timeout, urlErr.Timeout())
					if dns, ok := failure.cause.(*net.DNSError); ok {
						var recovered *net.DNSError
						require.ErrorAs(t, err, &recovered)
						require.Same(t, dns, recovered)
					}

					apiErr := openai_compatible.ErrorWrapper(err, "upstream_request_failed", http.StatusBadGateway)
					require.ErrorIs(t, apiErr.RawError, failure.cause)
					require.True(t, writeRelayFinalError(c, apiErr))
					require.Equal(t, http.StatusBadGateway, recorder.Code)
					require.Contains(t, recorder.Body.String(), "upstream_request_failed")
					lg.Error("transport failure fixture", zap.Error(apiErr.RawError))
					require.Contains(t, logs.String(), "transport failure fixture", "do not hide errors by removing diagnostics")
					for name, diagnostic := range map[string]string{
						"client_response": recorder.Body.String(), "zap_error": logs.String(),
						"error": err.Error(), "verbose_error": fmt.Sprintf("%+v", err),
					} {
						for _, private := range []string{secret, headerSecret, "fixture-private-transport-body", "fixture-unused-key"} {
							require.NotContains(t, diagnostic, private, name)
						}
					}
					require.Equal(t, wantURL, urlErr.URL)
					require.Contains(t, apiErr.Message, wantURL, "retain the useful URL, not just a generic failure")
				})
			}
		}
	}
}
