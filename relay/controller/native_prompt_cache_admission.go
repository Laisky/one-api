package controller

import (
	"encoding/json"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/pricing"
)

// errUnpricedPromptCacheWrite marks an admission refusal for explicit client
// cache writes in a catalog tier whose cache-write price is unpublished.
var errUnpricedPromptCacheWrite = errors.New("requested prompt-cache writes have no published tariff for this input size")

// promptCacheAdmission is one dispatch's admitted cache-write funding: the
// write buckets the tier quote must fund and whether the attempt must be sent
// with prompt_cache_options.mode="explicit".
type promptCacheAdmission struct {
	write5m, write1h bool
	pinExplicit      bool
}

// decideTierAdmissionCacheWrites computes the cache-write buckets a tiered
// admission must fund for one prepared dispatch without changing request
// state. It merges structural cache_control controls with OpenAI's native
// GPT-5.6+ prompt-cache contract, under which OpenAI writes the prompt cache
// implicitly unless the request disables it. For a catalog tier without a
// published cache-write price (for example GPT-6 Cyber above 272,000 input
// tokens) and no explicit operator tier, it refuses explicit client cache
// writes and otherwise requires prompt_cache_options.mode="explicit", so the
// upstream cannot create a write the gateway cannot price. Parameters: m is
// the attempt metadata, modelName the mapped model, payload the exact provider
// body, promptTokens the same input count the tier quote selects its tier
// with, configs the channel model configs and provider the pricing adaptor.
// It returns the decision, or a validation/refusal error.
func decideTierAdmissionCacheWrites(m *metalib.Meta, modelName string, payload any, promptTokens int,
	configs map[string]model.ModelConfigLocal, provider adaptor.Adaptor) (promptCacheAdmission, error) {
	write5m, write1h, err := tierAdmissionCacheWrites(payload)
	if err != nil {
		return promptCacheAdmission{}, err
	}
	decision := promptCacheAdmission{write5m: write5m, write1h: write1h}
	if m == nil || !openai.ImplicitPromptCacheWrites(m.ChannelType, modelName) {
		return decision, nil
	}
	raw, err := tierAdmissionPayloadJSON(payload)
	if err != nil {
		return promptCacheAdmission{}, err
	}
	intent, err := openai.InspectPromptCacheWriteIntent(raw)
	if err != nil {
		return promptCacheAdmission{}, errors.Wrap(err, "inspect native OpenAI prompt-cache controls")
	}
	policy, declared := openai.NativeModelBillingPolicy(m.ChannelType, modelName)
	cfg, _ := pricing.ResolveModelConfigRatioOnly(modelName, configs, provider, m.StartTime)
	if !declared || !pricing.CacheWriteUnpriced(policy, cfg, promptTokens, 0) {
		// The 30m lifetime bills through the legacy 5m write bucket.
		decision.write5m = decision.write5m || intent.Writes()
		return decision, nil
	}
	if intent.ClientRequestsWrites() {
		return promptCacheAdmission{}, adaptor.UnbillableRequestControl(errors.Wrapf(errUnpricedPromptCacheWrite,
			"%s publishes no cache-write price at %d input tokens; remove prompt_cache_breakpoint and prompt_cache_options mode/prewarm, shorten the input, or have the operator configure an explicit cache_write_5m_ratio tier",
			modelName, promptTokens))
	}
	// Explicit mode without breakpoints already creates no write.
	return promptCacheAdmission{write1h: write1h, pinExplicit: intent.Writes()}, nil
}

// recordPromptCacheAdmission stores one attempt's pin decision so the native
// OpenAI adaptor applies it at dispatch, clearing a previous attempt's
// decision. Parameters: c is the request context (nil for WebSocket frames,
// which cannot be rewritten), m the attempt metadata, and decision the
// admitted funding. It returns an error when a required pin cannot be applied.
func recordPromptCacheAdmission(c *gin.Context, m *metalib.Meta, decision promptCacheAdmission) error {
	if decision.pinExplicit && c == nil {
		return adaptor.UnbillableRequestControl(errors.Wrap(errUnpricedPromptCacheWrite, "implicit prompt caching cannot be disabled on this transport"))
	}
	openai.SetPromptCacheExplicitPin(c, m, decision.pinExplicit)
	if decision.pinExplicit {
		gmw.GetLogger(c).Debug("admission pins explicit prompt-cache mode: cache-write tier is unpublished",
			zap.String("model", m.ActualModelName), zap.Int("prompt_tokens", m.PromptTokens))
	}
	return nil
}

// tierAdmissionCacheWritesForDispatch decides and records one dispatch's
// cache-write funding (see decideTierAdmissionCacheWrites and
// recordPromptCacheAdmission). Parameters match decideTierAdmissionCacheWrites
// plus c, the request context. It returns the 5m and 1h write eligibility, or
// a validation/refusal error.
func tierAdmissionCacheWritesForDispatch(c *gin.Context, m *metalib.Meta, modelName string, payload any, promptTokens int,
	configs map[string]model.ModelConfigLocal, provider adaptor.Adaptor) (bool, bool, error) {
	decision, err := decideTierAdmissionCacheWrites(m, modelName, payload, promptTokens, configs, provider)
	if err != nil {
		openai.SetPromptCacheExplicitPin(c, m, false)
		return false, false, err
	}
	if err := recordPromptCacheAdmission(c, m, decision); err != nil {
		return false, false, err
	}
	return decision.write5m, decision.write1h, nil
}

// tierAdmissionPayloadJSON returns payload's JSON encoding without
// re-encoding exact provider bytes. Parameter payload is raw bytes or a
// parsed request. It returns the JSON or a wrapped encoding error.
func tierAdmissionPayloadJSON(payload any) ([]byte, error) {
	switch value := payload.(type) {
	case json.RawMessage:
		return value, nil
	case []byte:
		return value, nil
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.Wrap(err, "encode native prompt-cache admission payload")
	}
	return encoded, nil
}
