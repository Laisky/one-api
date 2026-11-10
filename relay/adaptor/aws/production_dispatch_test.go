package aws

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/one-api/common/ctxkey"
	channelmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/aws/utils"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/stretchr/testify/require"
)

// productionDispatchTransport records the dispatch marker at the real SDK HTTP boundary.
type productionDispatchTransport struct {
	h              *claudeInvokeHarness
	hits           int
	marked         bool
	status         int
	code           string
	transportError bool
}

// RoundTrip injects deterministic SDK errors without provider traffic.
func (p *productionDispatchTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	p.hits++
	p.marked = p.h.context.GetBool(ctxkey.UpstreamRequestPossiblyForwarded)
	if p.transportError {
		return nil, errors.WithStack(io.ErrUnexpectedEOF)
	}
	return &http.Response{StatusCode: p.status, Header: http.Header{"Content-Type": {"application/json"}, "X-Amzn-Errortype": {p.code}}, Body: io.NopCloser(strings.NewReader(`{"message":"fixture failure"}`)), Request: r}, nil
}

// TestClaudeProductionDispatchOwnership exercises production Init policy rather than a test-only retryer.
func TestClaudeProductionDispatchOwnership(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, tc := range []struct {
			name      string
			status    int
			code      string
			uncertain bool
			transport bool
		}{
			{"service_error", 503, "ServiceUnavailableException", true, false},
			{"transport_error", 0, "", true, true},
			{"access_denied", 403, "AccessDeniedException", false, false},
		} {
			t.Run(tc.name+map[bool]string{false: "/sync", true: "/stream"}[stream], func(t *testing.T) {
				h := newClaudeInvokeHarness(t, stream, nil, "us-east-1")
				h.meta.ActualModelName = "claude-fable-5-2"
				h.meta.Config = channelmodel.ChannelConfig{Region: "us-east-1", AK: "fixture-key", SK: "fixture-secret"}
				h.adaptor.Init(h.meta)
				require.NotNil(t, h.adaptor.AwsClient)
				opts := h.adaptor.AwsClient.Options()
				require.Equal(t, 1, opts.Retryer.MaxAttempts(), "production construction owns no-replay policy")
				tr := &productionDispatchTransport{h: h, status: tc.status, code: tc.code, transportError: tc.transport}
				opts.HTTPClient = &http.Client{Transport: tr}
				h.adaptor.AwsClient = bedrockruntime.New(opts)
				h.prepareChat(t)
				_, failure := h.adaptor.DoResponse(h.context, nil, h.meta)
				require.NotNil(t, failure)
				require.Equal(t, 1, tr.hits, "paid SDK calls must not replay")
				require.True(t, tr.marked, "forwarding evidence must precede SDK transport")
				require.Equal(t, tc.uncertain, h.context.GetBool(ctxkey.UpstreamRequestPossiblyForwarded))
				require.Equal(t, !tc.uncertain, utils.IsAdmissionRejection(failure))
			})
		}
	}
}
