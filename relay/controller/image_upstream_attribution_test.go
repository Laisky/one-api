package controller

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// imageAttributionSecret stands for private data a provider may put in an error
// body outside its documented message members; it must never reach the client.
const imageAttributionSecret = "internal-account-123"

// imageAttributionRedirect returns an HTTP client that sends every request to
// server whatever its host, so adaptors with a fixed provider host (Replicate)
// reach the fake provider.
//
// Parameters:
//   - server: the fake provider.
//
// Returns:
//   - *http.Client: the redirecting client.
func imageAttributionRedirect(server *httptest.Server) *http.Client {
	addr := server.Listener.Addr().String()
	return &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // test-only redirect to an httptest TLS server
	}}
}

// imageAttributionLogs loads the consume-log rows of one request.
//
// Parameters:
//   - t: the test.
//   - requestID: the request id.
//
// Returns:
//   - []model.Log: the rows, oldest first.
func imageAttributionLogs(t *testing.T, requestID string) []model.Log {
	t.Helper()
	var rows []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ?", requestID).Order("id").Find(&rows).Error)
	return rows
}

// TestImageUpstreamFailureAttributionAndSettlement drives the real image relay
// helper against a fake provider for the image adaptors that used to lose the
// provider's failure (xAI fabricated a 200 {"data":[]} or a local 500, Ali
// turned a 429 into a 500 one_api_error, Replicate a 500 with the raw body).
//
// Every failure must now be a provider-marked error with the provider's status
// and no raw body (a body without a recognized message gets the shared generic
// description). Settlement follows the image helper's documented policy
// (reconcileImageFailureBilling, TestImageUpstreamFailure_ForwardedChargeRemainsVisible):
// a provider 4xx is a rejection before inference and is refunded in full; a
// forwarded 5xx keeps the reservation as a visible "charge retained" ledger
// entry; a 2xx Ali answer with an error and no task is a rejected receipt and
// is refunded. No case reports a success to the client.
func TestImageUpstreamFailureAttributionAndSettlement(t *testing.T) {
	const balance = int64(100_000_000)
	cases := []struct {
		name, actual, body, contentType, upstream, wantMessage string
		channel, status, wantStatus                            int
		redirect, wantRetained                                 bool
	}{
		{name: "xai_429_flat", channel: channeltype.XAI, actual: "grok-imagine-image", body: `{"model":"alias","prompt":"p","n":1}`,
			status: 429, contentType: "application/json", upstream: `{"code":"AccountQuotaExceeded","error":"You have exceeded the monthly usage quota.","team":"` + imageAttributionSecret + `"}`,
			wantStatus: 429, wantMessage: "upstream returned HTTP 429: You have exceeded the monthly usage quota."},
		{name: "xai_503_json", channel: channeltype.XAI, actual: "grok-imagine-image", body: `{"model":"alias","prompt":"p","n":1}`,
			status: 503, contentType: "application/json", upstream: `{"error":"overloaded","host":"` + imageAttributionSecret + `"}`,
			wantStatus: 503, wantMessage: "upstream returned HTTP 503: overloaded", wantRetained: true},
		{name: "xai_503_text", channel: channeltype.XAI, actual: "grok-imagine-image", body: `{"model":"alias","prompt":"p","n":1}`,
			status: 503, contentType: "text/plain", upstream: `upstream unavailable ` + imageAttributionSecret,
			wantStatus: 503, wantRetained: true},
		{name: "ali_429", channel: channeltype.Ali, actual: "wanx-v1", body: `{"model":"alias","prompt":"p","n":1,"size":"1024*1024"}`,
			status: 429, contentType: "application/json", upstream: `{"code":"Throttling.RateQuota","message":"Requests rate limit exceeded.","request_id":"r1","account":"` + imageAttributionSecret + `"}`,
			wantStatus: 429, wantMessage: "Requests rate limit exceeded."},
		{name: "ali_503_html", channel: channeltype.Ali, actual: "wanx-v1", body: `{"model":"alias","prompt":"p","n":1,"size":"1024*1024"}`,
			status: 503, contentType: "text/html", upstream: `<html>` + imageAttributionSecret + `</html>`,
			wantStatus: 503, wantRetained: true},
		{name: "ali_200_error_without_task", channel: channeltype.Ali, actual: "wanx-v1", body: `{"model":"alias","prompt":"p","n":1,"size":"1024*1024"}`,
			status: 200, contentType: "application/json", upstream: `{"code":"InvalidParameter","message":"size is invalid","request_id":"r2"}`,
			wantStatus: 500, wantMessage: "ali async task failed: size is invalid"},
		{name: "replicate_429", channel: channeltype.Replicate, actual: "black-forest-labs/flux-1.1-pro", body: `{"model":"black-forest-labs/flux-1.1-pro","prompt":"p","n":1,"response_format":"b64_json"}`,
			status: 429, contentType: "application/problem+json", upstream: `{"title":"Too many requests","detail":"Request was throttled.","trace":"` + imageAttributionSecret + `"}`,
			wantStatus: 429, wantMessage: "upstream returned HTTP 429: Request was throttled.", redirect: true},
		{name: "replicate_503_html", channel: channeltype.Replicate, actual: "black-forest-labs/flux-1.1-pro", body: `{"model":"black-forest-labs/flux-1.1-pro","prompt":"p","n":1,"response_format":"b64_json"}`,
			status: 503, contentType: "text/html", upstream: `<html>` + imageAttributionSecret + `</html>`,
			wantStatus: 503, redirect: true, wantRetained: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			xaiVideoSetup(t, balance, false)
			var calls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.upstream)
			}))
			defer server.Close()
			previous := client.HTTPClient
			if tc.redirect {
				client.HTTPClient = imageAttributionRedirect(server)
			} else {
				client.HTTPClient = server.Client()
			}
			defer func() { client.HTTPClient = previous }()

			c, w, requestID := protocolContext(t, tc.channel, tc.actual, "/v1/images/generations", tc.body, server.URL, balance, 1, false, nil)
			apiErr := RelayImageHelper(c, relaymode.ImagesGenerations)
			drainCriticalTasks(t)

			require.EqualValues(t, 1, calls.Load(), "exactly one provider attempt")
			require.NotNil(t, apiErr, "a provider failure must never be reported as a success")
			require.Equal(t, tc.wantStatus, apiErr.StatusCode, "the provider status must be preserved")
			require.True(t, apiErr.FromUpstream, "the failure must be provider-marked")
			require.False(t, apiErr.IsOneAPIError(), "a provider failure must not read as one-api's own error")
			wantMessage := tc.wantMessage
			if wantMessage == "" {
				// No recognized message: the shared generic description applies.
				wantMessage = relaymodel.UpstreamErrorBodyMessage(tc.status, tc.contentType, []byte(tc.upstream))
				require.True(t, strings.HasPrefix(wantMessage, fmt.Sprintf("upstream returned HTTP %d with ", tc.status)))
			}
			require.Equal(t, wantMessage, apiErr.Message)
			clientJSON, err := json.Marshal(apiErr)
			require.NoError(t, err)
			require.NotContains(t, string(clientJSON), imageAttributionSecret, "raw provider body must not be echoed")
			require.Empty(t, w.Body.String(), "no fabricated image response may reach the client")

			quotaAfter := reloadUserQuota(t)
			cost := requestCostQuota(t, requestID)
			logs := imageAttributionLogs(t, requestID)
			require.NotEmpty(t, logs, "the reservation must leave a visible ledger row")
			last := logs[len(logs)-1]
			if tc.wantRetained {
				charged := balance - quotaAfter
				require.Positive(t, charged, "a forwarded 5xx keeps its reservation by policy")
				require.Equal(t, charged, cost, "request cost must match the retained debit")
				require.EqualValues(t, charged, last.Quota)
				require.Contains(t, last.Content, "charge retained")
				return
			}
			require.Equal(t, balance, quotaAfter, "a rejected generation must be refunded in full")
			require.Zero(t, cost)
			require.Zero(t, last.Quota)
			require.Contains(t, last.Content, "refunded")
		})
	}
}
