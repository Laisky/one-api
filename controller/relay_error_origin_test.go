package controller

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	rcontroller "github.com/Laisky/one-api/relay/controller"
	"github.com/Laisky/one-api/relay/model"
)

// upstreamErrorFromBody parses body exactly as the relay parses a provider's
// non-2xx response, so the result carries the parser's provenance.
func upstreamErrorFromBody(status int, body string) *model.ErrorWithStatusCode {
	return rcontroller.RelayErrorHandler(&http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))})
}

// TestRelayErrorOriginTrustBoundary pins the error-origin trust boundary from
// the PR #627 review. The same type/code pair must be classified as a caller or
// gateway fault only when one-api produced it locally; when a provider body
// carries it (nested or flat envelope), every classifier must treat it as the
// channel's failure: retry is not vetoed, the channel health counts it, and
// 5xx/delivery exemptions do not apply.
func TestRelayErrorOriginTrustBoundary(t *testing.T) {
	type verdicts struct{ userOriginated, adaptorInternal, countsHealth, delivery bool }
	for _, tc := range []struct {
		name          string
		local, remote *model.ErrorWithStatusCode
		localWant     verdicts
		remoteWant    verdicts
	}{
		{
			name:       "403_user_quota",
			local:      openai.ErrorWrapper(errors.New("user quota is not enough"), "insufficient_user_quota", http.StatusForbidden),
			remote:     upstreamErrorFromBody(http.StatusForbidden, `{"error":{"message":"user quota is not enough","type":"one_api_error","code":"insufficient_user_quota"}}`),
			localWant:  verdicts{userOriginated: true},
			remoteWant: verdicts{countsHealth: true},
		},
		{
			name:       "401_token_flat_envelope",
			local:      openai.ErrorWrapper(errors.New("token has expired"), "token_expired", http.StatusUnauthorized),
			remote:     upstreamErrorFromBody(http.StatusUnauthorized, `{"message":"token has expired","type":"one_api_error","code":"token_expired"}`),
			localWant:  verdicts{userOriginated: true},
			remoteWant: verdicts{countsHealth: true},
		},
		{
			name:       "400_validation",
			local:      openai.ErrorWrapper(errors.New("invalid request"), "invalid_request", http.StatusBadRequest),
			remote:     upstreamErrorFromBody(http.StatusBadRequest, `{"error":{"message":"invalid request","type":"one_api_error","code":"invalid_request"}}`),
			localWant:  verdicts{userOriginated: true},
			remoteWant: verdicts{},
		},
		{
			name:       "500_adaptor_fault",
			local:      openai.ErrorWrapper(errors.New("adaptor bug"), "do_request_failed", http.StatusInternalServerError),
			remote:     upstreamErrorFromBody(http.StatusInternalServerError, `{"error":{"message":"adaptor bug","type":"one_api_error","code":"do_request_failed"}}`),
			localWant:  verdicts{adaptorInternal: true},
			remoteWant: verdicts{countsHealth: true},
		},
		{
			name:       "502_delivery_code",
			local:      model.NewDeliveryError(errors.New("client went away"), model.DeliveryCodeStream),
			remote:     upstreamErrorFromBody(http.StatusBadGateway, `{"error":{"message":"client went away","type":"one_api_error","code":"`+model.DeliveryCodeStream+`"}}`),
			localWant:  verdicts{adaptorInternal: true, delivery: true},
			remoteWant: verdicts{countsHealth: true},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.False(t, tc.local.FromUpstream)
			require.True(t, tc.remote.FromUpstream, "the provider parser must mark its output")
			require.Equal(t, model.ErrorTypeOneAPI, tc.remote.Type, "the provider's type is still reported verbatim")
			for _, side := range []struct {
				err  *model.ErrorWithStatusCode
				want verdicts
			}{{tc.local, tc.localWant}, {tc.remote, tc.remoteWant}} {
				got := verdicts{
					userOriginated:  isUserOriginatedRelayError(side.err),
					adaptorInternal: isAdaptorInternalError(side.err),
					countsHealth:    countsAgainstChannelHealth(side.err),
					delivery:        model.IsDeliveryFailure(side.err),
				}
				require.Equal(t, side.want, got, "from_upstream=%t", side.err.FromUpstream)
			}
		})
	}
}
