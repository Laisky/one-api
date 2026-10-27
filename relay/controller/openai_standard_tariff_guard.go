package controller

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
)

const (
	openAICacheScanMaxDepth = 64
	openAICacheScanMaxNodes = 100_000
)

// normalizeOpenAITerraStandardPricing enforces the common unpriced-cache
// boundary for published native GPT-5.6/6 models and additionally pins Terra
// to the Standard service tier. It checks raw controls before root filtering.
func normalizeOpenAITerraStandardPricing(root map[string]json.RawMessage, model string, channel int) (bool, error) {
	if channel != channeltype.OpenAI {
		return false, nil
	}
	modelName := strings.ToLower(strings.TrimSpace(model))
	if requiresNativeOpenAICacheWriteTariff(modelName) {
		if err := rejectUnpricedNativeOpenAICacheLifetime(root); err != nil {
			return false, err
		}
	}
	if modelName != "gpt-6.1-terra" {
		return false, nil
	}

	const standard = `"default"`
	if current, ok := root["service_tier"]; ok && !bytes.Equal(bytes.TrimSpace(current), []byte("null")) {
		var requested string
		if err := json.Unmarshal(current, &requested); err != nil {
			return false, errors.Wrap(err, "decode Terra service_tier")
		}
		if !strings.EqualFold(strings.TrimSpace(requested), "default") {
			return false, errors.Errorf("GPT-6.1 Terra Standard pricing does not support service_tier %q", requested)
		}
	}
	if bytes.Equal(bytes.TrimSpace(root["service_tier"]), []byte(standard)) {
		return false, nil
	}
	root["service_tier"] = json.RawMessage(standard)
	return true, nil
}

// requiresNativeOpenAICacheWriteTariff selects published GPT-5.6+ models
// whose 12h cache-write tariff is not yet represented in the quota meter.
func requiresNativeOpenAICacheWriteTariff(name string) bool {
	if !strings.HasPrefix(name, "gpt-5.6") && !strings.HasPrefix(name, "gpt-6") {
		return false
	}
	config, known := openai.ModelRatios[name]
	return known && config.CacheWrite5mRatio > 0
}

// rejectUnpricedNativeOpenAICacheLifetime accepts only default or 30m cache
// writes on native OpenAI models with separate unmetered 12h prices.
func rejectUnpricedNativeOpenAICacheLifetime(root map[string]json.RawMessage) error {
	if raw, ok := root["prompt_cache_options"]; ok && len(raw) > 0 {
		// Preserve field presence: decoding JSON null into a Go string would
		// otherwise silently accept a malformed (and unaudited) cache TTL.
		var options struct {
			TTL json.RawMessage `json:"ttl"`
		}
		if err := json.Unmarshal(raw, &options); err != nil {
			return errors.Wrap(err, "decode OpenAI prompt_cache_options")
		}
		if len(options.TTL) > 0 {
			if bytes.Equal(bytes.TrimSpace(options.TTL), []byte("null")) {
				return errors.New("OpenAI cache-write TTL must be a string")
			}
			var ttl string
			if err := json.Unmarshal(options.TTL, &ttl); err != nil {
				return errors.Wrap(err, "decode OpenAI cache-write TTL as string")
			}
			if ttl = strings.TrimSpace(ttl); ttl != "" && !strings.EqualFold(ttl, "30m") {
				return errors.Errorf("OpenAI cache-write TTL %q requires a separately metered tariff", ttl)
			}
		}
	}

	for _, key := range []string{"input", "tools", "prompt"} {
		raw := root[key]
		if len(raw) == 0 {
			continue
		}
		var node any
		if err := json.Unmarshal(raw, &node); err != nil {
			return errors.Wrapf(err, "decode Terra %s cache breakpoints", key)
		}
		nodes := 0
		if err := inspectOpenAICacheBreakpoint(node, 0, &nodes); err != nil {
			return err
		}
	}
	return nil
}

// inspectOpenAICacheBreakpoint walks provider-owned input/tool structures,
// never user text or JSON schemas. Traversal is bounded against deep inputs.
func inspectOpenAICacheBreakpoint(value any, depth int, nodes *int) error {
	if depth > openAICacheScanMaxDepth {
		return errors.New("OpenAI cache breakpoint structure exceeds nesting limit")
	}
	*nodes++
	if *nodes > openAICacheScanMaxNodes {
		return errors.New("OpenAI cache breakpoint structure exceeds traversal limit")
	}
	switch item := value.(type) {
	case []any:
		for _, child := range item {
			if err := inspectOpenAICacheBreakpoint(child, depth+1, nodes); err != nil {
				return err
			}
		}
	case map[string]any:
		if raw, present := item["prompt_cache_breakpoint"]; present && raw != nil {
			point, ok := raw.(map[string]any)
			if !ok {
				return errors.New("OpenAI prompt_cache_breakpoint must be an object")
			}
			if rawTTL, present := point["ttl"]; present {
				ttl, ok := rawTTL.(string)
				if !ok {
					return errors.New("OpenAI cache breakpoint TTL must be a string")
				}
				if ttl = strings.TrimSpace(ttl); ttl != "" && !strings.EqualFold(ttl, "30m") {
					return errors.Errorf("OpenAI cache breakpoint TTL %q requires a separately metered tariff", ttl)
				}
			}
		}
		for key, child := range item {
			switch key {
			// Tool parameter schemas and prompt-template variables are caller
			// data, not provider cache controls. Scanning them previously
			// rejected valid requests whose data contained a TTL-like key.
			case "prompt_cache_breakpoint", "metadata", "arguments", "input_schema", "schema", "parameters", "variables", "data", "output", "extra_body":
				continue
			}
			if err := inspectOpenAICacheBreakpoint(child, depth+1, nodes); err != nil {
				return err
			}
		}
	}
	return nil
}
