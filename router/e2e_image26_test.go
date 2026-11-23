package router

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
)

// e2eImageModels are the published GPT Image 2.6 IDs.
var e2eImageModels = []string{"gpt-image-2.6-sunburst", "gpt-image-2.6-flare"}

// e2eImageBody is the client image generation request for modelName.
func e2eImageBody(modelName string) string {
	return `{"model":"` + modelName + `","prompt":"A lighthouse"}`
}

// e2eImageReply answers an Images API call with one image and receiptUsage
// as its usage object ("" omits usage).
func e2eImageReply(receiptUsage string) e2eReply {
	usage := ""
	if receiptUsage != "" {
		usage = `,"usage":` + receiptUsage
	}
	return e2eReply{Body: `{"created":1767225600,"data":[{"b64_json":"AQ=="}]` + usage + `}`}
}

// e2eRequireImageWire asserts the client received the generated image.
func e2eRequireImageWire(t *testing.T, res e2eResult) {
	t.Helper()
	require.Equal(t, http.StatusOK, res.Code, "body=%s", res.Body)
	root := e2eDecode(t, res.Body)
	data, _ := root["data"].([]any)
	require.Len(t, data, 1, "body=%s", res.Body)
	item, _ := data[0].(map[string]any)
	require.Equal(t, "AQ==", item["b64_json"], "body=%s", res.Body)
}

// e2eImageTariffConfigs renders a channel model_configs JSON for modelName
// with the given entry body ("" returns no configs).
func e2eImageTariffConfigs(modelName, entry string) string {
	if entry == "" {
		return ""
	}
	return `{"` + modelName + `":` + entry + `}`
}

const (
	// e2eImageOperatorEntry is a non-native operator tariff: $4/M input
	// (ratio 2), $40/M image output (completion ratio 10, 20 quota per output
	// token) and $0.04 per image as reservation/fallback (20,000 quota).
	e2eImageOperatorEntry = `{"ratio":2,"completion_ratio":10,"image":{"price_per_image_usd":0.04}}`
	// e2eImageOperatorReceipt is 10 text input and 100 output image tokens:
	// 10 x 2 + 100 x 20 = 2,020 at the operator tariff.
	e2eImageOperatorReceipt = `{"input_tokens":10,"output_tokens":100,"total_tokens":110,"input_tokens_details":{"text_tokens":10,"image_tokens":0}}`
	e2eImageOperatorCharge  = int64(2_020)
	// e2eImageOperatorReservation is ceil($0.04 x 500,000) for one image.
	e2eImageOperatorReservation = int64(20_000)
	// e2eImageOperatorDefaultHold is the calculator default without a
	// per-image price at the operator tariff: auto size and quality are the
	// calculator maximum of 23,719 output tokens at $40/M plus the 12-byte
	// prompt at $4/M = $0.948808, x 500,000 quota/USD.
	e2eImageOperatorDefaultHold = int64(474_404)
	// e2eImageNativeDefaultHold is the same default at OpenAI's native
	// tariff: 23,719 x $30/M + 12 x $5/M = $0.71163, x 500,000 quota/USD.
	e2eImageNativeDefaultHold = int64(355_815)
	// e2eImageNativeOperatorReceiptCharge prices e2eImageOperatorReceipt at
	// the native tariff: 10 x 2.5 + 100 x 15 = 1,525.
	e2eImageNativeOperatorReceiptCharge = int64(1_525)
)

// TestE2EOperatorTariffNativeOnlyImages drives GPT Image 2.6 generations on
// Azure through the shipped router. Without any channel price the request is
// refused before reservation with 503 image_billing_not_configured; a
// per-image price without a token tariff is reserved, refused at dispatch and
// refunded (503 model_pricing_not_configured); a complete operator token
// tariff reserves the per-image fallback, or without one the calculator
// default priced at that tariff, and settles the receipt at the channel's
// token prices; and an unpriced primary fails over to a priced secondary.
func TestE2EOperatorTariffNativeOnlyImages(t *testing.T) {
	for _, imageModel := range e2eImageModels {
		for _, tc := range []struct {
			name, primary, code string
			secondary           bool
		}{
			{name: "unpriced", code: "image_billing_not_configured"},
			{name: "token_price_only", primary: `{"ratio":2,"completion_ratio":10}`},
			{name: "image_price_only", primary: `{"image":{"price_per_image_usd":0.04}}`, code: "model_pricing_not_configured"},
			{name: "priced", primary: e2eImageOperatorEntry},
			{name: "unpriced_fails_over", secondary: true},
		} {
			t.Run(fmt.Sprintf("azure/%s/%s", imageModel, tc.name), func(t *testing.T) {
				channels := []e2eChannel{{ID: 939801, Type: channeltype.Azure, Key: e2eAzureKey, Priority: 10, BaseURL: e2eAzureBase,
					Models: imageModel, Config: e2eAzureConfig, ModelConfigs: e2eImageTariffConfigs(imageModel, tc.primary)}}
				if tc.secondary {
					channels = append(channels, e2eChannel{ID: 939802, Type: channeltype.Azure, Key: e2eAzureBackupKey, Priority: 5, BaseURL: e2eAzureBase,
						Models: imageModel, Config: e2eAzureConfig, ModelConfigs: e2eImageTariffConfigs(imageModel, e2eImageOperatorEntry)})
				}
				env := newE2EEnv(t, func(e2eCall) e2eReply { return e2eImageReply(e2eImageOperatorReceipt) }, channels...)
				before := env.ledger(t)

				res := env.post(t, "/v1/images/generations", e2eImageBody(imageModel))

				served := e2eAzureKey
				switch tc.name {
				case "unpriced_fails_over":
					require.Empty(t, env.upstream.callsFor(e2eAzureKey), "the unpriced primary must never be contacted")
					served = e2eAzureBackupKey
				case "priced", "token_price_only":
				default:
					errObject := e2eRequireErrorWire(t, e2eChat, res, http.StatusServiceUnavailable)
					require.Equal(t, tc.code, errObject["code"], "body=%s", res.Body)
					if tc.code == "image_billing_not_configured" {
						require.Contains(t, errObject["message"], "image.price_per_image_usd", "the refusal must tell the operator what to configure")
					}
					require.Zero(t, env.upstream.total(), "no provider may be contacted without a complete operator tariff")
					env.requireNothingCharged(t, res, before)
					return
				}
				e2eRequireImageWire(t, res)
				calls := env.upstream.callsFor(served)
				require.Len(t, calls, 1, "a priced image request is dispatched exactly once")
				require.Equal(t, "/openai/deployments/"+imageModel+"/images/generations", calls[0].Path)
				require.Equal(t, imageModel, calls[0].Body["model"])
				reserved := before.UserQuota - calls[0].UserQuota
				if tc.name == "token_price_only" {
					// Float64 pricing may round the derived hold up by one
					// quota unit ($0.000002), never below the exact price.
					require.GreaterOrEqual(t, reserved, e2eImageOperatorDefaultHold, "admission reserves the derived default")
					require.LessOrEqual(t, reserved-e2eImageOperatorDefaultHold, int64(1), "admission reserves the derived default")
				} else {
					require.Equal(t, e2eImageOperatorReservation, reserved, "admission reserves the operator per-image fallback")
				}
				require.Equal(t, "auto", calls[0].Body["size"], "catalog request defaults apply without an image tariff")
				require.Equal(t, "auto", calls[0].Body["quality"], "catalog request defaults apply without an image tariff")
				env.requireSettled(t, res, before, e2eImageOperatorCharge, false)
			})
		}
	}
}

// TestE2EOperatorTariffAIProxyNeverServesImages proves the AIProxy library,
// which has no image endpoint, is never routed a GPT Image 2.6 request even
// when it is the only channel listing the model with a complete tariff: no
// provider is contacted and nothing is charged.
func TestE2EOperatorTariffAIProxyNeverServesImages(t *testing.T) {
	for _, imageModel := range e2eImageModels {
		t.Run(imageModel, func(t *testing.T) {
			env := newE2EEnv(t, func(e2eCall) e2eReply { return e2eImageReply(e2eImageOperatorReceipt) },
				e2eChannel{ID: 939811, Type: channeltype.AIProxyLibrary, Key: e2eAIProxyKey, BaseURL: e2eAIProxyBase, Models: imageModel,
					Config: e2eAIProxyConfig, ModelConfigs: e2eImageTariffConfigs(imageModel, e2eImageOperatorEntry)})
			before := env.ledger(t)

			res := env.post(t, "/v1/images/generations", e2eImageBody(imageModel))

			// The distributor excludes the channel by endpoint, so selection finds none.
			errObject := e2eRequireErrorWire(t, e2eChat, res, http.StatusServiceUnavailable)
			require.Contains(t, errObject["message"], "No available channels for Model "+imageModel, "body=%s", res.Body)
			require.Zero(t, env.upstream.total(), "the AIProxy library must never be routed an image request")
			env.requireNothingCharged(t, res, before)
		})
	}
}

// e2eNativeImageCase is one native OpenAI GPT Image 2.6 receipt.
type e2eNativeImageCase struct {
	name    string
	usage   string
	charge  int64
	flagged bool
}

// e2eNativeImageCases price receipts by hand at OpenAI's native GPT Image 2.6
// token tariff, five buckets: text input $5/M (2.5 quota per token), cached
// text $1.25/M (0.625), image input $8/M (4), cached image $2/M (1) and
// output $30/M (15). The operator sets only image.price_per_image_usd = 0.1,
// whose reservation is 50,000 quota for one image.
var e2eNativeImageCases = []e2eNativeImageCase{
	{
		// 800 text x 2.5 = 2,000; 200 cached text x 0.625 = 125; 400 image x 4
		// = 1,600; 100 cached image x 1 = 100; 200 output x 15 = 3,000.
		name:   "five_bucket_receipt",
		usage:  `{"input_tokens":1500,"output_tokens":200,"total_tokens":1700,"input_tokens_details":{"text_tokens":1000,"image_tokens":500,"cached_tokens":300,"cached_tokens_details":{"text_tokens":200,"image_tokens":100}}}`,
		charge: 6_825,
	},
	{
		// The cache split is omitted, so the 400 cached tokens are credited to
		// the text bucket (the smallest discount): 600 text x 2.5 = 1,500; 400
		// cached text x 0.625 = 250; 500 image x 4 = 2,000; 200 output x 15 =
		// 3,000. The largest consistent charge, flagged as an upper bound.
		name:    "omitted_cache_split_upper_bound",
		usage:   `{"input_tokens":1500,"output_tokens":200,"total_tokens":1700,"input_tokens_details":{"text_tokens":1000,"image_tokens":500,"cached_tokens":400}}`,
		charge:  6_750,
		flagged: true,
	},
	{
		// No receipt: the per-image fallback is charged, flagged.
		name: "no_receipt_fallback", charge: 50_000, flagged: true,
	},
}

// TestE2EImage26NativeOpenAI drives GPT Image 2.6 generations on a native
// OpenAI channel. Without a positive image.price_per_image_usd the request
// reserves the calculator default (auto size and quality at the effective
// token tariff), settles a token receipt by the five token buckets and
// refunds the excess, or keeps the hold when no receipt arrives; with the
// price, the per-image fallback is reserved instead and settled the same way.
func TestE2EImage26NativeOpenAI(t *testing.T) {
	for _, imageModel := range e2eImageModels {
		for _, entry := range []struct {
			name, configs, usage string
			charge               int64
			flagged              bool
		}{
			{name: "no_model_configs", usage: e2eImageOperatorReceipt, charge: e2eImageNativeOperatorReceiptCharge},
			{name: "token_tariff_only", configs: `{"ratio":2.5,"completion_ratio":6}`, usage: e2eImageOperatorReceipt, charge: e2eImageNativeOperatorReceiptCharge},
			{name: "zero_image_price", configs: `{"image":{"price_per_image_usd":0}}`, usage: e2eImageOperatorReceipt, charge: e2eImageNativeOperatorReceiptCharge},
			{name: "no_receipt_keeps_default_hold", charge: e2eImageNativeDefaultHold, flagged: true},
		} {
			t.Run(fmt.Sprintf("%s/default_reservation/%s", imageModel, entry.name), func(t *testing.T) {
				env := newE2EEnv(t, func(e2eCall) e2eReply { return e2eImageReply(entry.usage) },
					e2eNativeChannel(939821, e2eNativeKey, 0, imageModel, e2eImageTariffConfigs(imageModel, entry.configs)))
				before := env.ledger(t)

				res := env.post(t, "/v1/images/generations", e2eImageBody(imageModel))

				e2eRequireImageWire(t, res)
				calls := env.upstream.callsFor(e2eNativeKey)
				require.Len(t, calls, 1, "a default-reserved image request is dispatched exactly once")
				require.Equal(t, imageModel, calls[0].Body["model"])
				require.Equal(t, e2eImageNativeDefaultHold, before.UserQuota-calls[0].UserQuota, "admission reserves the calculator default")
				env.requireSettled(t, res, before, entry.charge, entry.flagged)
			})
		}
		for _, tc := range e2eNativeImageCases {
			t.Run(fmt.Sprintf("%s/priced/%s", imageModel, tc.name), func(t *testing.T) {
				env := newE2EEnv(t, func(e2eCall) e2eReply { return e2eImageReply(tc.usage) },
					e2eNativeChannel(939831, e2eNativeKey, 0, imageModel, e2eImageTariffConfigs(imageModel, `{"image":{"price_per_image_usd":0.1}}`)))
				before := env.ledger(t)

				res := env.post(t, "/v1/images/generations", e2eImageBody(imageModel))

				e2eRequireImageWire(t, res)
				calls := env.upstream.callsFor(e2eNativeKey)
				require.Len(t, calls, 1)
				require.Equal(t, "/v1/images/generations", calls[0].Path)
				require.Equal(t, imageModel, calls[0].Body["model"])
				// ceil($0.1 x 500,000) for one image at the default size/quality.
				require.EqualValues(t, 50_000, before.UserQuota-calls[0].UserQuota, "admission reserves the per-image fallback")
				env.requireSettled(t, res, before, tc.charge, tc.flagged)
			})
		}
	}
}

// TestE2EImage26FreeGroupIsServedFree pins a free group (group ratio 0, which
// makes a channel free for that group) on GPT Image 2.6. The configured
// per-image fallback must not turn a free group into a refusal: the request is
// served and nothing is charged. Defect: validateStrictGPTImageBilling
// required a positive group ratio and answered 503 "requires a positive
// image.price_per_image_usd" although the price was configured.
func TestE2EImage26FreeGroupIsServedFree(t *testing.T) {
	previous := billingratio.GroupRatio2JSONString()
	require.NoError(t, billingratio.UpdateGroupRatioByJSONString(`{"default":0}`))
	t.Cleanup(func() { require.NoError(t, billingratio.UpdateGroupRatioByJSONString(previous)) })
	for _, imageModel := range e2eImageModels {
		t.Run(imageModel, func(t *testing.T) {
			env := newE2EEnv(t, func(e2eCall) e2eReply { return e2eImageReply(e2eNativeImageCases[0].usage) },
				e2eNativeChannel(939841, e2eNativeKey, 0, imageModel, e2eImageTariffConfigs(imageModel, `{"image":{"price_per_image_usd":0.1}}`)))
			before := env.ledger(t)

			res := env.post(t, "/v1/images/generations", e2eImageBody(imageModel))

			e2eRequireImageWire(t, res)
			require.Len(t, env.upstream.callsFor(e2eNativeKey), 1)
			env.requireNothingCharged(t, res, before)
		})
	}
}
