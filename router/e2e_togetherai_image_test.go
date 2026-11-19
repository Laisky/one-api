package router

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
)

const (
	// e2eTogetherKey is the credential of the Together AI fixture channel.
	e2eTogetherKey = "e2e-together-key"
	// e2eTogetherBase is Together AI's production base URL.
	e2eTogetherBase = "https://api.together.xyz"
	// Together bills these four "per megapixel" (https://www.together.ai/pricing).
	e2eTogetherFluxMax     = "black-forest-labs/FLUX.2-max"
	e2eTogetherKontextPro  = "black-forest-labs/FLUX.1-kontext-pro"
	e2eTogetherKontextMax  = "black-forest-labs/FLUX.1-kontext-max"
	e2eTogetherFlux11Pro   = "black-forest-labs/FLUX.1.1-pro"
	e2eTogetherFluxDev     = "black-forest-labs/FLUX.2-dev"
	e2eTogetherFluxFlex    = "black-forest-labs/FLUX.2-flex"
	e2eTogetherGPTImage2   = "openai/gpt-image-2"
	e2eTogetherGeminiImage = "google/gemini-3-pro-image"
)

// e2eTogetherImageEnv builds a gateway with one Together AI channel serving
// its FLUX models plus flat and tiered controls, and a fake upstream that
// returns one image per call. Parameters: t owns cleanup. Returns: the env.
func e2eTogetherImageEnv(t *testing.T) *e2eEnv {
	t.Helper()
	return newE2EEnv(t, func(e2eCall) e2eReply { return e2eImageReply("") },
		e2eChannel{ID: 939871, Type: channeltype.TogetherAI, Key: e2eTogetherKey, BaseURL: e2eTogetherBase,
			Models: e2eTogetherFluxMax + "," + e2eTogetherKontextPro + "," + e2eTogetherKontextMax + "," +
				e2eTogetherFlux11Pro + "," + e2eTogetherFluxDev + "," + e2eTogetherFluxFlex + "," +
				e2eTogetherGPTImage2 + "," + e2eTogetherGeminiImage})
}

// TestE2ETogetherImageBillsForwardedDimensionsAndSteps proves a Together AI
// image generation is charged for what Together renders and bills. Together's
// Images API has no size field and renders the width and height it receives
// (https://docs.together.ai/reference/post-images-generations), so the gateway
// forwards the billed dimensions as width/height. Per-megapixel models cost
// "MP x Price per MP x (Steps / Default Steps)" with "MP = (Width x Height /
// 1,000,000)", steps counting only above the default
// (https://docs.together.ai/docs/serverless/models.md). Per-image models
// list their lowest-resolution price ("Displayed prices refer to the lowest
// resolution/duration settings"), which is therefore a floor. Quota is
// $1 = 500,000 and admission must reserve exactly the settled charge.
func TestE2ETogetherImageBillsForwardedDimensionsAndSteps(t *testing.T) {
	for _, tc := range []struct {
		name, body    string
		width, height float64
		steps         any
		charge        int64
	}{
		// 1024x1024 = 1.048576 MP x $0.07.
		{"max_default", `{"model":"` + e2eTogetherFluxMax + `","prompt":"a fox"}`, 1024, 1024, nil, 36_701},
		// Raw width/height decide the output: 3.6864 MP x $0.07 = $0.258048.
		{"max_raw_width_height", `{"model":"` + e2eTogetherFluxMax + `","prompt":"a fox","width":1920,"height":1920}`, 1920, 1920, nil, 129_024},
		// An OpenAI size is forwarded as width/height: 0.262144 MP x $0.07.
		{"max_size_forwarded", `{"model":"` + e2eTogetherFluxMax + `","prompt":"a fox","size":"512x512"}`, 512, 512, nil, 9_176},
		// 60 of 50 default steps: 1.048576 x $0.07 x 1.2 = $0.0880803840.
		{"max_steps_above_default", `{"model":"` + e2eTogetherFluxMax + `","prompt":"a fox","steps":60}`, 1024, 1024, float64(60), 44_041},
		// Fewer steps never reduce the price.
		{"max_steps_below_default", `{"model":"` + e2eTogetherFluxMax + `","prompt":"a fox","steps":20}`, 1024, 1024, float64(20), 36_701},
		// 2.0736 MP x $0.04 x 56/28 = $0.165888.
		{"kontext_pro_size_and_steps", `{"model":"` + e2eTogetherKontextPro + `","prompt":"a fox","width":1440,"height":1440,"steps":56,"image_url":"https://example.com/a.png"}`, 1440, 1440, float64(56), 82_944},
		// 1.048576 MP x $0.08.
		{"kontext_max_default", `{"model":"` + e2eTogetherKontextMax + `","prompt":"a fox"}`, 1024, 1024, nil, 41_944},
		// 1440x1024 = 1.47456 MP x $0.04; FLUX1.1 [pro] lists no default steps.
		{"flux11_pro_raw_dimensions", `{"model":"` + e2eTogetherFlux11Pro + `","prompt":"a fox","width":1440,"height":1024}`, 1440, 1024, nil, 29_492},
		// A partial pair keeps the configured default for the missing side.
		{"flux11_pro_width_only", `{"model":"` + e2eTogetherFlux11Pro + `","prompt":"a fox","width":1440}`, 1440, 1024, nil, 29_492},
		// Per-image models never bill below the listed lowest-resolution price.
		{"dev_small_size_floor", `{"model":"` + e2eTogetherFluxDev + `","prompt":"a fox","size":"512x512"}`, 512, 512, nil, 7_700},
		{"flex_small_size_floor", `{"model":"` + e2eTogetherFluxFlex + `","prompt":"a fox","size":"512x512"}`, 512, 512, nil, 15_000},
		// Above one megapixel they keep scaling with area: 2.0736 MP x $0.0154.
		{"dev_raw_large_dimensions", `{"model":"` + e2eTogetherFluxDev + `","prompt":"a fox","width":1920,"height":1080,"steps":40}`, 1920, 1080, float64(40), 15_967},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := e2eTogetherImageEnv(t)
			before := env.ledger(t)

			res := env.post(t, "/v1/images/generations", tc.body)

			e2eRequireImageWire(t, res)
			calls := env.upstream.callsFor(e2eTogetherKey)
			require.Len(t, calls, 1)
			require.Equal(t, "/v1/images/generations", calls[0].Path)
			require.Equal(t, tc.width, calls[0].Body["width"], "Together renders the forwarded width; raw=%s", calls[0].Raw)
			require.Equal(t, tc.height, calls[0].Body["height"], "Together renders the forwarded height; raw=%s", calls[0].Raw)
			require.Equal(t, tc.steps, calls[0].Body["steps"], "steps are forwarded unchanged")
			require.Equal(t, tc.charge, before.UserQuota-calls[0].UserQuota, "admission must reserve the settled charge")
			env.requireSettled(t, res, before, tc.charge, false)
		})
	}
}

// TestE2ETogetherImageForwardsClientFields proves the rewritten Together body
// keeps every client field it does not price, such as reference images.
func TestE2ETogetherImageForwardsClientFields(t *testing.T) {
	env := e2eTogetherImageEnv(t)
	res := env.post(t, "/v1/images/generations", `{"model":"`+e2eTogetherFluxFlex+
		`","prompt":"a fox","reference_images":["https://example.com/a.png"],"guidance_scale":5,"seed":7,"n":1}`)

	e2eRequireImageWire(t, res)
	calls := env.upstream.callsFor(e2eTogetherKey)
	require.Len(t, calls, 1)
	require.Equal(t, e2eTogetherFluxFlex, calls[0].Body["model"])
	require.Equal(t, "a fox", calls[0].Body["prompt"])
	require.Equal(t, []any{"https://example.com/a.png"}, calls[0].Body["reference_images"])
	require.Equal(t, float64(5), calls[0].Body["guidance_scale"])
	require.Equal(t, float64(7), calls[0].Body["seed"])
}

// TestE2ETogetherImageRefusesUnpricedControls proves requests whose rendered
// size or step count cannot be priced are refused before any reservation or
// provider call.
func TestE2ETogetherImageRefusesUnpricedControls(t *testing.T) {
	for _, tc := range []struct {
		name, body, code string
	}{
		{"size_disagrees_with_width", `{"model":"` + e2eTogetherFluxMax + `","prompt":"a fox","size":"512x512","width":1920}`, "size_not_supported"},
		{"aspect_ratio", `{"model":"` + e2eTogetherKontextPro + `","prompt":"a fox","aspect_ratio":"21:9"}`, "invalid_image_request"},
		{"fractional_width", `{"model":"` + e2eTogetherFluxMax + `","prompt":"a fox","width":1024.5,"height":1024}`, "invalid_image_request"},
		{"zero_height", `{"model":"` + e2eTogetherFluxMax + `","prompt":"a fox","width":1024,"height":0}`, "invalid_image_request"},
		{"string_steps", `{"model":"` + e2eTogetherFluxMax + `","prompt":"a fox","steps":"60"}`, "invalid_image_request"},
		{"zero_steps", `{"model":"` + e2eTogetherKontextPro + `","prompt":"a fox","steps":0}`, "invalid_image_request"},
		{"unmeasurable_size", `{"model":"` + e2eTogetherFluxDev + `","prompt":"a fox","size":"auto"}`, "size_not_supported"},
		// Parsers disagree on case-folded and repeated keys, so the priced
		// value might not be the one Together reads.
		{"case_folded_width", `{"model":"` + e2eTogetherFluxMax + `","prompt":"a fox","width":512,"Width":1920}`, "invalid_image_request"},
		{"repeated_width", `{"model":"` + e2eTogetherFluxMax + `","prompt":"a fox","width":512,"width":1920}`, "invalid_image_request"},
		{"case_folded_steps", `{"model":"` + e2eTogetherFluxMax + `","prompt":"a fox","STEPS":500}`, "invalid_image_request"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := e2eTogetherImageEnv(t)
			before := env.ledger(t)

			res := env.post(t, "/v1/images/generations", tc.body)

			require.Equal(t, http.StatusBadRequest, res.Code, "body=%s", res.Body)
			require.Contains(t, res.Body, tc.code)
			require.Zero(t, env.upstream.total(), "an unpriced request must never reach the provider")
			env.requireNothingCharged(t, res, before)
		})
	}
}

// TestE2ETogetherImageRefusesMultipart proves a non-JSON body, whose width and
// height the gateway cannot price, never reaches Together's JSON-only API.
func TestE2ETogetherImageRefusesMultipart(t *testing.T) {
	env := e2eTogetherImageEnv(t)
	before := env.ledger(t)
	body, contentType := e2eImageEditForm(t,
		map[string]string{"model": e2eTogetherFluxMax, "prompt": "a fox", "width": "1920", "height": "1920"}, nil)

	res := env.postMultipart(t, "/v1/images/generations", body, contentType)

	require.Equal(t, http.StatusBadRequest, res.Code, "body=%s", res.Body)
	require.Contains(t, res.Body, "invalid_image_request")
	require.Contains(t, res.Body, "must be a JSON object")
	require.Zero(t, env.upstream.total())
	env.requireNothingCharged(t, res, before)
}

// TestE2ETogetherImageFlatAndTieredControls proves Together models whose
// tariff does not depend on dimensions keep their flat charge and an unchanged
// body, and that Gemini 3 Pro Image is billed by the documented tier of the
// width and height Together renders.
func TestE2ETogetherImageFlatAndTieredControls(t *testing.T) {
	for _, tc := range []struct {
		name, body    string
		width, height any
		charge        int64
	}{
		// GPT Image 2 is a flat $0.053; no dimensions are injected.
		{"flat_unchanged", `{"model":"` + e2eTogetherGPTImage2 + `","prompt":"a fox"}`, nil, nil, 26_500},
		// 1K and 2K cost $0.134; the default is the 1K square.
		{"gemini_default_1k", `{"model":"` + e2eTogetherGeminiImage + `","prompt":"a fox"}`, float64(1024), float64(1024), 67_000},
		{"gemini_2k", `{"model":"` + e2eTogetherGeminiImage + `","prompt":"a fox","width":2752,"height":1536}`, float64(2752), float64(1536), 67_000},
		// 4K costs about $0.24.
		{"gemini_4k", `{"model":"` + e2eTogetherGeminiImage + `","prompt":"a fox","width":5504,"height":3072}`, float64(5504), float64(3072), 120_000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := e2eTogetherImageEnv(t)
			before := env.ledger(t)

			res := env.post(t, "/v1/images/generations", tc.body)

			e2eRequireImageWire(t, res)
			calls := env.upstream.callsFor(e2eTogetherKey)
			require.Len(t, calls, 1)
			require.Equal(t, tc.width, calls[0].Body["width"], "raw=%s", calls[0].Raw)
			require.Equal(t, tc.height, calls[0].Body["height"], "raw=%s", calls[0].Raw)
			require.Equal(t, tc.charge, before.UserQuota-calls[0].UserQuota, "admission must reserve the settled charge")
			env.requireSettled(t, res, before, tc.charge, false)
		})
	}
}
