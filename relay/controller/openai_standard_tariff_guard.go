package controller

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/channeltype"
)

const (
	openAITerraCacheScanMaxDepth = 64
	openAITerraCacheScanMaxNodes = 100_000
)

// normalizeOpenAITerraStandardPricing restricts the new native Terra model to
// the Standard pricing contract verified by this gateway. It inspects the raw
// provider request before unknown roots are dropped and pins service_tier so
// upstream project defaults cannot silently select an unmetered paid tier.
// Parameters: root is the mutable wire object, model is mapped upstream, and
// channel identifies its provider. Returns: whether it changed the wire and
// any unsupported paid-tariff control error.
func normalizeOpenAITerraStandardPricing(root map[string]json.RawMessage, model string, channel int) (bool, error) {
	if channel != channeltype.OpenAI || !strings.EqualFold(strings.TrimSpace(model), "gpt-6.1-terra") {
		return false, nil
	}
	if err := rejectOpenAITerraTwelveHourCache(root); err != nil {
		return false, err
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

// rejectOpenAITerraTwelveHourCache prevents accepting a 12h cache-write rate
// without its independent token bucket. The top-level options are examined even
// when the current provider-root manifest would subsequently omit them.
// Parameters: root is the original request JSON object. Returns: an error for
// unsupported TTL selections or malformed structural cache controls.
func rejectOpenAITerraTwelveHourCache(root map[string]json.RawMessage) error {
	if raw, ok := root["prompt_cache_options"]; ok && len(raw) > 0 {
		var options struct {
			TTL string `json:"ttl"`
		}
		if err := json.Unmarshal(raw, &options); err != nil {
			return errors.Wrap(err, "decode Terra prompt_cache_options")
		}
		if strings.EqualFold(strings.TrimSpace(options.TTL), "12h") {
			return errors.New("GPT-6.1 Terra 12h cache writes require a separately metered tariff")
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
		if err := inspectOpenAITerraCacheBreakpoint(node, 0, &nodes); err != nil {
			return err
		}
	}
	return nil
}

// inspectOpenAITerraCacheBreakpoint walks provider-owned message/tool structure,
// not user text or embedded schemas, and rejects expensive breakpoint lifetimes.
// Parameters: value is a decoded structural subtree, depth is its nesting level,
// and nodes counts inspected elements. Returns: an error on unpriced or unsafe
// structures and nil for ordinary 30m/default cache controls.
func inspectOpenAITerraCacheBreakpoint(value any, depth int, nodes *int) error {
	if depth > openAITerraCacheScanMaxDepth {
		return errors.New("Terra cache breakpoint structure exceeds nesting limit")
	}
	*nodes++
	if *nodes > openAITerraCacheScanMaxNodes {
		return errors.New("Terra cache breakpoint structure exceeds traversal limit")
	}
	switch item := value.(type) {
	case []any:
		for _, child := range item {
			if err := inspectOpenAITerraCacheBreakpoint(child, depth+1, nodes); err != nil {
				return err
			}
		}
	case map[string]any:
		if raw, ok := item["prompt_cache_breakpoint"]; ok {
			if point, ok := raw.(map[string]any); ok {
				if ttl, ok := point["ttl"].(string); ok && strings.EqualFold(strings.TrimSpace(ttl), "12h") {
					return errors.New("GPT-6.1 Terra 12h cache breakpoints require a separately metered tariff")
				}
			}
		}
		for key, child := range item {
			switch key {
			case "prompt_cache_breakpoint", "metadata", "arguments", "input_schema", "schema", "data", "output", "extra_body":
				continue
			}
			if err := inspectOpenAITerraCacheBreakpoint(child, depth+1, nodes); err != nil {
				return err
			}
		}
	}
	return nil
}
