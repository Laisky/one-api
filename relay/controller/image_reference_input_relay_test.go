package controller

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
)

// strictImageEditBalance is the starting user and token quota of the edit
// billing cases: large enough for every hold and receipt below.
const strictImageEditBalance = int64(10_000_000)

// strictImageMultipartEdit builds a multipart GPT Image edit carrying refs
// reference images of edge x edge pixels and, when withMask is set, a mask.
// Parameters: t owns the fixture, modelName is the mapped model, n the image
// count, refs the reference count, edge the reference size, and withMask
// whether a mask part is sent. Returns: the body and its Content-Type.
func strictImageMultipartEdit(t *testing.T, modelName string, n, refs, edge int, withMask bool) ([]byte, string) {
	t.Helper()
	parts := []testMultipartPart{
		{name: "model", content: []byte(modelName)},
		{name: "prompt", content: []byte("A lighthouse")},
		{name: "n", content: []byte(fmt.Sprint(n))},
		{name: "size", content: []byte("1024x1024")},
		{name: "quality", content: []byte("high")},
	}
	for i := range refs {
		parts = append(parts, testMultipartPart{name: "image[]", filename: fmt.Sprintf("ref-%d.png", i), content: encodeTestImage(t, "png", edge, edge)})
	}
	if withMask {
		parts = append(parts, testMultipartPart{name: "mask", filename: "mask.png", content: encodeTestImage(t, "png", edge, edge)})
	}
	body, boundary := buildTestMultipart(t, parts...)
	return body, "multipart/form-data; boundary=" + boundary
}

// TestStrictGPTImageEditReferenceInputBilling drives GPT Image 2.5/2.6 edits
// through RelayImageHelper, the SQLite ledger, and a fake provider. OpenAI
// bills every reference image (and mask) as image input tokens and publishes
// no rule that bounds them, so the calculator-derived default (output tokens
// plus prompt bytes) cannot cover an edit: without an explicit per-image
// reservation/fallback the edit is refused before reservation and before any
// provider call. With one, the existing reconciliation holds: an unusable
// receipt keeps the hold, an incomplete one is charged at least its own
// ceiling, a complete one settles its exact token price (refunding or debiting
// the difference), and an unaffordable hold never reaches the provider.
// Parameters: t runs the cases. Returns: none.
func TestStrictGPTImageEditReferenceInputBilling(t *testing.T) {
	const priced = `{%q:{"image":{"price_per_image_usd":0.2}}}`
	// 0.2 USD at 500,000 quota/USD is a 100,000 quota hold per image.
	const perImageHold = int64(100_000)

	t.Run("unpriced_edits_are_refused_before_upstream", func(t *testing.T) {
		jsonEdit := []byte(`{"model":"gpt-image-2.6-sunburst","prompt":"A lighthouse","size":"1024x1024","quality":"high",` +
			`"images":[{"image_url":"data:image/png;base64,iVBORw0KGgo="},{"image_url":"https://example.localhost/ref.png"}]}`)
		multipartBody, multipartType := strictImageMultipartEdit(t, "gpt-image-2.5-sunburst", 1, 4, 2048, true)
		for _, tc := range []struct {
			name, model, contentType, configs string
			channelType                       int
			body                              []byte
		}{
			{name: "native_multipart_multiple_large_references_and_mask", model: "gpt-image-2.5-sunburst", contentType: multipartType, body: multipartBody},
			{name: "native_json_reference_urls", model: "gpt-image-2.6-sunburst", contentType: "application/json", body: jsonEdit},
			{name: "compatible_verbatim_multipart", model: "gpt-image-2.5-sunburst", contentType: multipartType, body: multipartBody, channelType: channeltype.OpenAICompatible},
			{
				// A complete token tariff still derives only an output estimate.
				name: "token_tariff_without_per_image_price", model: "gpt-image-2.5-sunburst", contentType: multipartType, body: multipartBody,
				configs: `{"gpt-image-2.5-sunburst":{"ratio":5,"completion_ratio":4}}`,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				res := runStrictImageRelay(t, strictImageRelayCase{
					requestID: "strict-image-edit-refused-" + tc.name, channelType: tc.channelType, model: tc.model,
					path: "/v1/images/edits", contentType: tc.contentType, body: tc.body, configs: tc.configs,
					usage: `{"input_tokens":1,"output_tokens":1,"total_tokens":2}`, balance: strictImageEditBalance,
				})
				requireStrictImageRefusedBeforeUpstream(t, res, strictImageEditBalance, http.StatusServiceUnavailable, "image_billing_not_configured")
				require.Contains(t, res.err.Message, "reference images", "the refusal must name what the default cannot bound")
				require.Contains(t, res.err.Message, "image.price_per_image_usd", "the refusal must name the setting that admits the edit")
			})
		}
	})

	t.Run("generation_without_references_keeps_the_derived_default", func(t *testing.T) {
		// Control: the same unconfigured channel still admits a generation.
		res := runStrictImageRelay(t, strictImageRelayCase{
			requestID: "strict-image-generation-default-control", model: "gpt-image-2.5-sunburst",
			path: "/v1/images/generations", contentType: "application/json",
			body:  []byte(`{"model":"gpt-image-2.5-sunburst","prompt":"A lighthouse","size":"1024x1024","quality":"high"}`),
			usage: `null`, balance: strictImageEditBalance,
		})
		hold := expectedDefaultImageHold(1756, 30, 5, len("A lighthouse"), 1)
		requireStrictImageSettled(t, res, strictImageEditBalance, hold, hold, "default_fallback")
	})

	for _, tc := range []struct {
		name, model, usage, source string
		n, refs, edge              int
		mask                       bool
		hold, charge               int64
		channelType                int
	}{
		{
			// No usable receipt: the operator's explicit fallback is charged.
			name: "missing_receipt_keeps_hold", model: "gpt-image-2.5-sunburst", usage: `null`,
			n: 2, refs: 3, edge: 2048, mask: true, hold: 2 * perImageHold, charge: 2 * perImageHold, source: "configured_fallback",
		},
		{
			// Output only, no input count: at least the receipt's own ceiling,
			// 20,000 output tokens at $30/M = $0.6, above the $0.2 hold.
			name: "incomplete_receipt_charges_its_ceiling", model: "gpt-image-2.6-flare", usage: `{"output_tokens":20000}`,
			n: 1, refs: 2, edge: 1536, hold: perImageHold, charge: 300_000, source: "provider_tokens_upper_bound",
		},
		{
			// An incomplete receipt below the hold keeps the hold.
			name: "incomplete_receipt_below_hold_keeps_hold", model: "gpt-image-2.6-flare", usage: `{"output_tokens":1000}`,
			n: 1, refs: 1, edge: 1024, hold: perImageHold, charge: perImageHold, source: "configured_fallback",
		},
		{
			// 100 text x $5 + 1,000 image x $8 + 1,000 output x $30 = $0.0385.
			name: "complete_receipt_refunds_excess_hold", model: "gpt-image-2.5-sunburst",
			usage: `{"input_tokens":1100,"output_tokens":1000,"total_tokens":2100,"input_tokens_details":{"text_tokens":100,"image_tokens":1000}}`,
			n:     1, refs: 1, edge: 1024, hold: perImageHold, charge: 19_250, source: "provider_tokens",
		},
		{
			// Large references: 100 text x $5 + 39,900 image x $8 + 5,000
			// output x $30 = $0.4697, debited beyond the $0.2 hold.
			name: "complete_receipt_with_large_references_debits_beyond_hold", model: "gpt-image-2.5-sunburst",
			usage: `{"input_tokens":40000,"output_tokens":5000,"total_tokens":45000,"input_tokens_details":{"text_tokens":100,"image_tokens":39900}}`,
			n:     1, refs: 16, edge: 2048, hold: perImageHold, charge: 234_850, source: "provider_tokens",
		},
		{
			// The verbatim OpenAI-compatible body settles the same way.
			name: "compatible_missing_receipt_keeps_hold", model: "gpt-image-2.5-flare", usage: `null`, channelType: channeltype.OpenAICompatible,
			n: 1, refs: 2, edge: 2048, mask: true, hold: perImageHold, charge: perImageHold, source: "configured_fallback",
		},
	} {
		t.Run("priced_"+tc.name, func(t *testing.T) {
			body, contentType := strictImageMultipartEdit(t, tc.model, tc.n, tc.refs, tc.edge, tc.mask)
			res := runStrictImageRelay(t, strictImageRelayCase{
				requestID: "strict-image-edit-" + tc.name, channelType: tc.channelType, model: tc.model,
				path: "/v1/images/edits", contentType: contentType, body: body,
				configs: fmt.Sprintf(priced, tc.model), usage: tc.usage, balance: strictImageEditBalance,
			})
			requireStrictImageSettled(t, res, strictImageEditBalance, tc.hold, tc.charge, tc.source)
			require.Equal(t, body, res.sentBody, "a native or compatible multipart edit is forwarded verbatim")
		})
	}

	t.Run("priced_insufficient_quota_never_reaches_upstream", func(t *testing.T) {
		const balance = int64(50_000) // below the 100,000 hold
		body, contentType := strictImageMultipartEdit(t, "gpt-image-2.5-sunburst", 1, 2, 2048, false)
		res := runStrictImageRelay(t, strictImageRelayCase{
			requestID: "strict-image-edit-insufficient", model: "gpt-image-2.5-sunburst",
			path: "/v1/images/edits", contentType: contentType, body: body,
			configs: fmt.Sprintf(priced, "gpt-image-2.5-sunburst"), usage: `null`, balance: balance,
		})
		requireStrictImageRefusedBeforeUpstream(t, res, balance, http.StatusForbidden, "insufficient_user_quota")
	})
}
