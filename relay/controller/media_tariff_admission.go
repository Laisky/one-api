package controller

import (
	"encoding/json"
	"net/http"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
)

// mediaTariffAdmission resolves catalog media contracts before quota reservation.
// Parameters: c and info identify the authenticated request and fallback is its
// existing token/input quote. Returns: the correct single-generation quote, the
// unchanged quote for other units, or an error before any paid provider dispatch.
func mediaTariffAdmission(c *gin.Context, info *meta.Meta, fallback int64) (int64, *relaymodel.ErrorWithStatusCode) {
	provider := resolvePricingAdaptor(info)
	base, found := pricing.ResolveModelConfig(info.ActualModelName, nil, provider, info.StartTime)
	if !found || base.PricingProvenance == nil {
		return fallback, nil
	}
	_, overrides := getChannelModelPricingFromContext(c)
	cfg, _ := pricing.ResolveModelConfig(info.ActualModelName, overrides, provider, info.StartTime)
	if err := pricing.ValidateTariffProvenance(cfg, info.StartTime); err != nil {
		return 0, openai.ErrorWrapper(err, "unresolved_media_tariff", http.StatusBadRequest)
	}
	tariff, generation, err := pricing.ResolveGenerationTariff(info.ActualModelName, overrides, provider, info.StartTime)
	if err != nil {
		return 0, openai.ErrorWrapper(err, "unresolved_media_tariff", http.StatusBadRequest)
	}
	if !generation {
		return fallback, nil
	}
	if err := validateSingleGenerationRequest(c); err != nil {
		return 0, openai.ErrorWrapper(err, "unbounded_generation_request", http.StatusBadRequest)
	}
	quote, err := pricing.GenerationQuota(tariff, c.GetFloat64(ctxkey.ChannelRatio))
	if err != nil {
		return 0, openai.ErrorWrapper(err, "invalid_generation_tariff", http.StatusBadRequest)
	}
	c.Set(mediaGenerationAdmissionKey, generationAdmission{model: info.ActualModelName, channel: info.ChannelId, token: info.TokenId, user: info.UserId, quota: quote})
	return quote, nil
}

// validateSingleGenerationRequest rejects caller batching and tool loops for a
// one-generation tariff. Parameters: c retains the original bounded JSON body.
// Returns: an error for unsupported multiplicity, or nil for an ordinary single
// generation. Both root and extra_body are checked before provider conversion.
func validateSingleGenerationRequest(c *gin.Context) error {
	value, ok := c.Get(ctxkey.KeyRequestBody)
	if !ok {
		return errors.New("generation request body is unavailable")
	}
	raw, ok := value.([]byte)
	if !ok {
		return errors.New("generation request body has an invalid type")
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return errors.Wrap(err, "decode generation request")
	}
	objects := []map[string]json.RawMessage{root}
	if raw := root["extra_body"]; len(raw) > 0 {
		var extra map[string]json.RawMessage
		if err := json.Unmarshal(raw, &extra); err != nil {
			return errors.Wrap(err, "decode generation extra body")
		}
		objects = append(objects, extra)
	}
	for _, fields := range objects {
		if value := fields["n"]; len(value) > 0 {
			var n int
			if err := json.Unmarshal(value, &n); err != nil || n != 1 {
				return errors.New("generation tariff supports exactly one result per request")
			}
		}
		for _, name := range []string{"candidate_count", "candidateCount", "num_outputs", "num_generations", "function_call", "tool_choice"} {
			if _, exists := fields[name]; exists {
				return errors.New("unsupported generation multiplicity or tool operation")
			}
		}
		for _, name := range []string{"tools", "functions"} {
			if value := fields[name]; len(value) > 0 {
				var entries []json.RawMessage
				if err := json.Unmarshal(value, &entries); err != nil || len(entries) > 0 {
					return errors.New("generation tariff does not support tool loops")
				}
			}
		}
	}
	return nil
}

const mediaGenerationAdmissionKey = "one_api.media_generation_admission"

// generationAdmission binds uncertain-work settlement to one authorized model,
// channel, owner, token and reserved amount, preventing reuse across retries.
type generationAdmission struct {
	model                string
	channel, token, user int
	quota                int64
}

// isRetainedGenerationAdmission reports whether this exact request admission is
// a validated generation. Parameters: c holds the marker, info is current relay
// metadata and amount is its retained hold. Returns: true only for matching work.
func isRetainedGenerationAdmission(c *gin.Context, info *meta.Meta, amount int64) bool {
	value, exists := c.Get(mediaGenerationAdmissionKey)
	if !exists || info == nil {
		return false
	}
	admission, ok := value.(generationAdmission)
	return ok && admission.model == info.ActualModelName && admission.channel == info.ChannelId && admission.token == info.TokenId && admission.user == info.UserId && admission.quota == amount
}
