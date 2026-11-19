package controller

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
)

// normalizeNativeOpenAIStandardTier enforces native OpenAI billing policies on
// a raw Responses body before root filtering. For GPT-5.6+ models whose 30m
// cache writes are metered it validates the prompt-cache controls (rejecting
// unmetered lifetimes such as 12h) and forwards a sanitized
// prompt_cache_options, so a client's explicit-only cache mode is honored;
// every other channel or model keeps the historical behavior of never
// forwarding prompt_cache_options. It pins models whose policy restricts
// service tiers to the policy's supported tier. Parameters are the decoded
// root object (mutated in place), the mapped model ID and the relay channel
// type. It returns whether root changed, or an admission error.
func normalizeNativeOpenAIStandardTier(root map[string]json.RawMessage, model string, channel int) (bool, error) {
	modelName := strings.ToLower(strings.TrimSpace(model))
	changed := false
	if !openai.ImplicitPromptCacheWrites(channel, modelName) {
		if _, present := root[promptCacheOptionsRootKey]; present {
			delete(root, promptCacheOptionsRootKey)
			changed = true
		}
		if channel != channeltype.OpenAI {
			return changed, nil
		}
	} else {
		if _, err := openai.InspectPromptCacheWriteIntentRoot(root); err != nil {
			return false, errors.Wrap(err, "validate native OpenAI prompt-cache controls")
		}
		if raw, present := root[promptCacheOptionsRootKey]; present {
			sanitized, keep, err := openai.SanitizePromptCacheOptions(raw)
			if err != nil {
				return false, errors.Wrap(err, "sanitize native OpenAI prompt_cache_options")
			}
			switch {
			case !keep:
				delete(root, promptCacheOptionsRootKey)
				changed = true
			case !bytes.Equal(bytes.TrimSpace(raw), sanitized):
				root[promptCacheOptionsRootKey] = sanitized
				changed = true
			}
		}
	}
	policy, declared := openai.NativeModelBillingPolicy(channel, modelName)
	if !declared || !policy.RestrictsServiceTier() {
		return changed, nil
	}

	pinned, encodeErr := json.Marshal(policy.PinnedServiceTier())
	if encodeErr != nil {
		return false, errors.Wrap(encodeErr, "encode OpenAI pinned service_tier")
	}
	if current, ok := root["service_tier"]; ok && !bytes.Equal(bytes.TrimSpace(current), []byte("null")) {
		var requested string
		if err := json.Unmarshal(current, &requested); err != nil {
			return false, errors.Wrap(err, "decode OpenAI Standard service_tier")
		}
		if !policy.SupportsServiceTier(requested) {
			return false, adaptor.UnbillableRequestControl(errors.Errorf("%s Standard pricing does not support service_tier %q", modelName, requested))
		}
	}
	if bytes.Equal(bytes.TrimSpace(root["service_tier"]), pinned) {
		return changed, nil
	}
	root["service_tier"] = json.RawMessage(pinned)
	return true, nil
}

// promptCacheOptionsRootKey is OpenAI's root prompt-cache control. The native
// Responses root allow-list admits it because normalizeNativeOpenAIStandardTier
// removes it from every request whose cache writes this gateway cannot meter.
const promptCacheOptionsRootKey = "prompt_cache_options"
