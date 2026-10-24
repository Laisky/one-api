package controller

import (
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	quotautil "github.com/Laisky/one-api/relay/quota"
)

// prepareResponseTierBody normalizes and serializes the native provider body
// once before tier admission. Parameters: c holds raw input, meta and request
// contain admitted mapped controls, and provider is the native adaptor.
// Returns: exact dispatch bytes or the wrapped preparation error.
func prepareResponseTierBody(c *gin.Context, meta *metalib.Meta, request *openai.ResponseAPIRequest, provider adaptor.Adaptor) ([]byte, error) {
	body, err := getResponseAPIRequestBody(c, meta, request, provider)
	if err != nil {
		return nil, errors.Wrap(err, "prepare tiered Responses provider body")
	}
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, errors.Wrap(err, "read tiered Responses provider body")
	}
	return data, nil
}

// quoteResponseTierAdmission prices only active tier contracts against the
// actual prepared native request. Parameters: c and meta supply effective
// pricing, request and promptTokens retain standalone helper compatibility, and
// prepared optionally supplies exact provider bytes. Returns: a complete tier
// allowance, whether tiers apply, or a wrapped invalid-pricing/body error.
func quoteResponseTierAdmission(c *gin.Context, meta *metalib.Meta, request *openai.ResponseAPIRequest, promptTokens int, prepared []byte) (int64, bool, error) {
	if c == nil || meta == nil || request == nil {
		return 0, false, errors.WithStack(errors.New("Responses tier quote requires request metadata"))
	}
	channelRatios, configs := getChannelModelPricingFromContext(c)
	var completionRatios map[string]float64
	if value, exists := c.Get(ctxkey.ChannelModel); exists {
		if channel, ok := value.(*model.Channel); ok && channel != nil {
			completionRatios = channel.GetCompletionRatioFromConfigsWithContext(gmw.Ctx(c))
		}
	}
	provider := resolvePricingAdaptor(meta)
	cfg, known := pricing.ResolveModelConfigRatioOnly(request.Model, configs, provider, meta.StartTime)
	if !known || len(cfg.Tiers) == 0 {
		return 0, false, nil
	}
	quoted := request
	payload := any(request)
	if len(prepared) > 0 {
		quoted = &openai.ResponseAPIRequest{}
		if err := json.Unmarshal(prepared, quoted); err != nil {
			return 0, true, errors.Wrap(err, "decode prepared Responses tier controls")
		}
		if quoted.Model != request.Model {
			return 0, true, errors.WithStack(errors.New("prepared Responses pricing model differs from admitted model"))
		}
		promptTokens = getResponseAPIPromptTokens(gmw.Ctx(c), quoted)
		payload = json.RawMessage(prepared)
	}
	maxOutput := 0
	if quoted.MaxOutputTokens != nil {
		maxOutput = *quoted.MaxOutputTokens
	}
	write5m, write1h, err := tierAdmissionCacheWrites(payload)
	if err != nil {
		return 0, true, errors.Wrap(err, "inspect prepared Responses cache controls")
	}
	groupRatio := c.GetFloat64(ctxkey.ChannelRatio)
	budgetPromptTokens := promptTokens
	// An authoritative free group needs no inherited token allowance. Ordinary
	// owner-binding authorization has already completed before this quote.
	if groupRatio != 0 {
		budgetPromptTokens, err = responseTierAdmissionPromptBudget(quoted, promptTokens, configs, provider, meta.StartTime)
		if err != nil {
			return 0, true, errors.Wrap(err, "budget provider-resolved Responses input")
		}
		budgetPromptTokens, err = responseContinuationTierPrompt(c, meta, quoted, budgetPromptTokens)
		if err != nil {
			return 0, true, errors.Wrap(err, "budget owned Responses continuation")
		}
	}
	modelRatio := pricing.ResolveModelRatioAt(quoted.Model, configs, channelRatios, provider, meta.StartTime)
	quote, applies, err := quotautil.EstimateTierAdmission(quotautil.ComputeInput{
		Usage: &relaymodel.Usage{PromptTokens: budgetPromptTokens}, ModelName: quoted.Model,
		ModelRatio: modelRatio, ChannelModelRatio: channelRatios, GroupRatio: groupRatio,
		ChannelModelConfigs: configs, ChannelCompletionRatio: completionRatios,
		PricingAdaptor: provider, RequestTime: meta.StartTime,
	}, maxOutput, 0, quotautil.AdmissionOptions{CacheWrite5m: write5m, CacheWrite1h: write1h})
	if err != nil {
		return 0, applies, errors.Wrap(err, "quote complete Responses tier allowance")
	}
	meta.PromptTokens = promptTokens
	return quote, applies, nil
}

// responseTierAdmissionPromptBudget supplies a provider-enforced context-window
// allowance for input that cannot be tokenized locally (remote prompt templates
// and file-backed Responses content). It is shared by HTTP and WebSocket tier
// admission. Ordinary text/multimodal inputs keep their existing token estimates.
//
// If neither model metadata nor its upstream/default pricing catalog supplies
// a context bound, the gateway cannot guarantee a paid hold before dispatch.
// Such opaque paid-tier inputs are rejected instead of silently underfunded.
func responseTierAdmissionPromptBudget(request *openai.ResponseAPIRequest, estimated int, configs map[string]model.ModelConfigLocal, provider adaptor.Adaptor, at time.Time) (int, error) {
	if request == nil || estimated < 0 {
		return 0, errors.New("invalid Responses tier prompt estimate")
	}
	if !responseTierHasOpaqueProviderInput(request) {
		return estimated, nil
	}
	effective, _ := pricing.ResolveModelConfig(request.Model, configs, provider, at)
	upstream, _ := pricing.ResolveModelConfig(request.Model, nil, provider, at)
	// A channel-specific context override is an administrative estimate, not
	// proof that the upstream will reject larger inputs. Prefer the higher
	// upstream-published bound where it is available.
	bound := max(int(effective.ContextLength), int(upstream.ContextLength))
	if bound <= 0 {
		return 0, errors.New("cannot safely price provider-resolved Responses input without a model context bound")
	}
	return max(estimated, bound), nil
}

// responseTierHasOpaqueProviderInput identifies input content whose actual token
// count depends on provider-side expansion instead of the client wire bytes.
// Unknown provider extensions are forwarded unchanged; no field filtering.
func responseTierHasOpaqueProviderInput(request *openai.ResponseAPIRequest) bool {
	if request == nil {
		return false
	}
	if request.Prompt != nil && strings.TrimSpace(request.Prompt.Id) != "" {
		return true
	}
	return responseTierContainsFileInput([]any(request.Input))
}

func responseTierContainsFileInput(value any) bool {
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			if responseTierContainsFileInput(item) {
				return true
			}
		}
	case map[string]any:
		if kind, _ := v["type"].(string); strings.EqualFold(kind, "input_file") {
			return true
		}
		for _, item := range v {
			if responseTierContainsFileInput(item) {
				return true
			}
		}
	case json.RawMessage:
		var decoded any
		if json.Unmarshal(v, &decoded) == nil {
			return responseTierContainsFileInput(decoded)
		}
	}
	return false
}
