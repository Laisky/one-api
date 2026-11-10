package aws

import (
	"math"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/one-api/common/ctxkey"
	channelmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	awsClaude "github.com/Laisky/one-api/relay/adaptor/aws/claude"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/gin-gonic/gin"
)

var _ adaptor.RequestPricingValidator = (*Adaptor)(nil)

// ValidateRequestPricing requires operator tariffs when a Claude 5.2 override changes automatic routing.
// Opaque application-profile ARNs cannot prove Global tariff eligibility; all explicit overrides
// require complete prices without rewriting existing administrator configuration or legacy tariffs.
func (a *Adaptor) ValidateRequestPricing(c *gin.Context, m *meta.Meta) error {
	if m == nil || c == nil {
		return errors.New("AWS pricing requires request metadata")
	}
	name := m.ActualModelName
	if name == "" {
		name = m.OriginModelName
	}
	if !anthropic.IsClaude52(name) {
		return nil
	}
	value, _ := c.Get(ctxkey.ChannelModel)
	channel, ok := value.(*channelmodel.Channel)
	if !ok || channel == nil {
		return nil
	}
	// Inspect both the current invocation key and the canonical key set by conversion.
	override := awsClaude.AwsClaudeModelTransArn(c, nil)
	if strings.TrimSpace(override) == "" {
		override = channel.GetInferenceProfileArnMapWithContext(gmw.Ctx(c))[name]
	}
	if strings.TrimSpace(override) == "" {
		return nil
	}
	if err := pricing.RequireExplicitTokenPricing(c, m); err != nil {
		return err
	}
	cfg, found := pricing.ResolveModelConfigRatioOnly(name, channel.GetModelPriceConfigsWithContext(gmw.Ctx(c)), nil, m.StartTime)
	valid := func(v float64) bool { return v > 0 && !math.IsNaN(v) && !math.IsInf(v, 0) }
	if !found || !valid(cfg.CachedInputRatio) || !valid(cfg.CacheWrite5mRatio) || !valid(cfg.CacheWrite1hRatio) {
		return errors.New("AWS Claude 5.2 profile override requires explicit positive cache read, 5m write, and 1h write pricing")
	}
	for _, tier := range cfg.Tiers {
		for _, v := range []float64{tier.CachedInputRatio, tier.CacheWrite5mRatio, tier.CacheWrite1hRatio} {
			if v != 0 && !valid(v) {
				return errors.New("AWS Claude 5.2 profile override has invalid explicit tier cache pricing")
			}
		}
	}
	return nil
}
