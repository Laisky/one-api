package controller

import (
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	relayadaptor "github.com/Laisky/one-api/relay/adaptor"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestResolveMetaUserAPIFormatReportsClientFormat proves the consume log
// records the API format the client used: a Responses client served through
// the Chat Completions fallback (meta.Mode switched to ChatCompletions) is
// logged as a Responses request, not as a Chat request.
func TestResolveMetaUserAPIFormatReportsClientFormat(t *testing.T) {
	responses := resolveUserAPIFormat(relaymode.ResponseAPI)
	require.NotEmpty(t, responses)
	require.Equal(t, responses, resolveMetaUserAPIFormat(&metalib.Meta{Mode: relaymode.ChatCompletions, ResponseAPIFallback: true}))
	require.Equal(t, resolveUserAPIFormat(relaymode.ChatCompletions), resolveMetaUserAPIFormat(&metalib.Meta{Mode: relaymode.ChatCompletions}))
	require.Equal(t, resolveUserAPIFormat(relaymode.ClaudeMessages), resolveMetaUserAPIFormat(&metalib.Meta{Mode: relaymode.ClaudeMessages}))
	require.Empty(t, resolveMetaUserAPIFormat(nil))
}

// TestDoRequestRefundReasonNamesLocalRefusals proves a refunded attempt's log
// row names a local tariff or control refusal instead of a generic dispatch
// failure, so operators can tell configuration gaps from transport errors.
func TestDoRequestRefundReasonNamesLocalRefusals(t *testing.T) {
	require.Equal(t, "request_pricing_not_configured",
		doRequestRefundReason(errors.Wrap(relayadaptor.RequestPricingNotConfigured(errors.New("no tariff")), "validate request pricing")))
	require.Equal(t, "request_control_refused",
		doRequestRefundReason(errors.Wrap(relayadaptor.UnbillableRequestControl(errors.New("no write tariff")), "apply policy")))
	require.Equal(t, "do_request_failed", doRequestRefundReason(errors.New("connection reset")))
}
