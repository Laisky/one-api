package openai

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/channeltype"
)

// OpenAI's GPT-5.6+ prompt-cache contract (verified 2026-11-18):
//   - Caching is on by default: without prompt_cache_options.mode="explicit"
//     OpenAI places one implicit breakpoint and bills the written prefix as a
//     30m cache write (1.25x uncached input).
//   - prompt_cache_options {mode, ttl, prewarm, comparison_response_id} is a
//     root request field; prompt_cache_breakpoint {mode, ttl} marks explicit
//     breakpoints on content parts (including function_call_output output).
//   - mode="explicit" without any explicit breakpoint disables caching, so the
//     request creates no cache write.
//   - ttl "12h" writes bill 2.5x input and are not metered by this gateway.
//
// Sources:
//   - https://developers.openai.com/api/docs/guides/prompt-caching.md
//   - https://developers.openai.com/api/reference/resources/responses/methods/create.md
const (
	promptCacheOptionsKey    = "prompt_cache_options"
	promptCacheBreakpointKey = "prompt_cache_breakpoint"
	promptCacheModeExplicit  = "explicit"
	promptCacheModeImplicit  = "implicit"
	promptCacheTTLDefault    = "30m"

	promptCacheScanMaxDepth = 64
	promptCacheScanMaxNodes = 100_000
)

// promptCacheBreakpointRoots lists the provider-owned root fields that can
// carry explicit breakpoints. Other roots hold caller data or scalar controls.
var promptCacheBreakpointRoots = []string{"input", "tools", "prompt"}

// PromptCacheWriteIntent summarizes how one native OpenAI request can create
// billed prompt-cache writes. The zero value describes a request without any
// cache control, which OpenAI caches implicitly.
type PromptCacheWriteIntent struct {
	// OptionsPresent reports that the request carries root prompt_cache_options.
	OptionsPresent bool
	// ExplicitMode reports prompt_cache_options.mode="explicit", which disables
	// OpenAI's implicit breakpoint.
	ExplicitMode bool
	// ImplicitModeRequested reports that the client explicitly asked for the
	// default implicit mode.
	ImplicitModeRequested bool
	// Prewarm reports prompt_cache_options.prewarm=true, a write-only request.
	Prewarm bool
	// Breakpoints counts explicit prompt_cache_breakpoint controls.
	Breakpoints int
}

// Writes reports whether OpenAI may bill a cache write for the request. It
// takes no parameters and returns true unless explicit mode is selected
// without any explicit breakpoint.
func (i PromptCacheWriteIntent) Writes() bool {
	return !i.ExplicitMode || i.Breakpoints > 0
}

// ClientRequestsWrites reports whether the client explicitly asked for cache
// writes rather than inheriting OpenAI's implicit default. It takes no
// parameters and returns true for explicit breakpoints, an explicit implicit
// mode, or prewarm.
func (i PromptCacheWriteIntent) ClientRequestsWrites() bool {
	return i.Breakpoints > 0 || i.ImplicitModeRequested || i.Prewarm
}

// ImplicitPromptCacheWrites reports whether the native OpenAI catalog meters
// cache writes that OpenAI creates implicitly for modelName (GPT-5.6 and later
// models with a published cache-write price). Parameters are the relay channel
// type and the mapped model ID. It returns false for every non-native channel,
// whose operator tariff and cache semantics are host-specific.
func ImplicitPromptCacheWrites(channelType int, modelName string) bool {
	if channelType != channeltype.OpenAI {
		return false
	}
	return meteredPromptCacheModel(modelName)
}

// meteredPromptCacheModel reports whether modelName is a GPT-5.6+ catalog
// entry whose 30m cache-write tariff is metered. Parameter modelName is a
// requested or mapped model ID; matching ignores case and surrounding spaces.
func meteredPromptCacheModel(modelName string) bool {
	name := normalizedModelName(modelName)
	if !strings.HasPrefix(name, "gpt-5.6") && !strings.HasPrefix(name, "gpt-6") {
		return false
	}
	config, known := ModelRatios[name]
	return known && config.CacheWrite5mRatio > 0
}

// InspectPromptCacheWriteIntent validates the native prompt-cache controls of
// a JSON request body and summarizes its write intent. Parameter body is the
// serialized provider payload. It returns the intent, or an error for a
// malformed control, an unsupported mode, or a TTL other than the metered
// default 30m lifetime.
func InspectPromptCacheWriteIntent(body []byte) (PromptCacheWriteIntent, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return PromptCacheWriteIntent{}, errors.Wrap(err, "decode OpenAI prompt-cache request")
	}
	if root == nil {
		return PromptCacheWriteIntent{}, errors.New("OpenAI prompt-cache request must be a JSON object")
	}
	return InspectPromptCacheWriteIntentRoot(root)
}

// InspectPromptCacheWriteIntentRoot is InspectPromptCacheWriteIntent for an
// already decoded root object. Parameter root is not modified. It returns the
// intent or a validation error marked adaptor.ErrUnbillableRequestControl.
func InspectPromptCacheWriteIntentRoot(root map[string]json.RawMessage) (PromptCacheWriteIntent, error) {
	intent, err := inspectPromptCacheWriteIntentRoot(root)
	if err != nil {
		return PromptCacheWriteIntent{}, adaptor.UnbillableRequestControl(err)
	}
	return intent, nil
}

// inspectPromptCacheWriteIntentRoot implements InspectPromptCacheWriteIntentRoot
// without marking errors. Parameter root is not modified. It returns the
// intent or the unmarked validation error.
func inspectPromptCacheWriteIntentRoot(root map[string]json.RawMessage) (PromptCacheWriteIntent, error) {
	var intent PromptCacheWriteIntent
	if raw, ok := root[promptCacheOptionsKey]; ok && !isJSONNull(raw) {
		options, err := decodePromptCacheOptions(raw)
		if err != nil {
			return PromptCacheWriteIntent{}, err
		}
		intent.OptionsPresent = true
		intent.ExplicitMode = options.mode == promptCacheModeExplicit
		intent.ImplicitModeRequested = options.mode == promptCacheModeImplicit
		intent.Prewarm = options.prewarm
	}
	for _, key := range promptCacheBreakpointRoots {
		raw := root[key]
		if len(raw) == 0 {
			continue
		}
		var node any
		if err := json.Unmarshal(raw, &node); err != nil {
			return PromptCacheWriteIntent{}, errors.Wrapf(err, "decode OpenAI %s cache breakpoints", key)
		}
		nodes := 0
		if err := scanPromptCacheBreakpoints(node, 0, &nodes, &intent.Breakpoints); err != nil {
			return PromptCacheWriteIntent{}, err
		}
	}
	return intent, nil
}

// promptCacheOptions is the validated, normalized subset of root
// prompt_cache_options that this gateway can meter.
type promptCacheOptions struct {
	mode     string
	ttl      string
	prewarm  bool
	hasTTL   bool
	hasMode  bool
	hasPrewm bool
}

// decodePromptCacheOptions validates root prompt_cache_options. Parameter raw
// is the field's JSON value. It returns the normalized options or an error for
// a non-object value, a non-string or unsupported mode or TTL, or a non-boolean
// prewarm flag.
func decodePromptCacheOptions(raw json.RawMessage) (promptCacheOptions, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return promptCacheOptions{}, errors.New("OpenAI prompt_cache_options must be an object")
	}
	var options promptCacheOptions
	if value, ok := fields["mode"]; ok {
		mode, err := promptCacheString(value, "prompt_cache_options.mode")
		if err != nil {
			return promptCacheOptions{}, err
		}
		switch mode = strings.ToLower(mode); mode {
		case "", promptCacheModeImplicit, promptCacheModeExplicit:
			options.mode, options.hasMode = mode, mode != ""
		default:
			return promptCacheOptions{}, errors.Errorf("OpenAI prompt_cache_options.mode %q is not supported", mode)
		}
	}
	if value, ok := fields["ttl"]; ok {
		ttl, err := promptCacheString(value, "cache-write TTL")
		if err != nil {
			return promptCacheOptions{}, err
		}
		if err := validatePromptCacheTTL(ttl, "OpenAI cache-write TTL"); err != nil {
			return promptCacheOptions{}, err
		}
		options.ttl, options.hasTTL = strings.ToLower(ttl), ttl != ""
	}
	if value, ok := fields["prewarm"]; ok && !isJSONNull(value) {
		if err := json.Unmarshal(value, &options.prewarm); err != nil {
			return promptCacheOptions{}, errors.New("OpenAI prompt_cache_options.prewarm must be a boolean")
		}
		options.hasPrewm = true
	}
	return options, nil
}

// SanitizePromptCacheOptions returns the forwardable form of root
// prompt_cache_options: only the metered mode, ttl and prewarm controls are
// kept, normalized to lower case. Diagnostics such as comparison_response_id
// reference provider response IDs outside this gateway's ownership model and
// are dropped. Parameter raw is the field's JSON value. It returns the
// sanitized JSON, false when nothing forwardable remains, or a validation error.
func SanitizePromptCacheOptions(raw json.RawMessage) (json.RawMessage, bool, error) {
	if isJSONNull(raw) {
		return nil, false, nil
	}
	options, err := decodePromptCacheOptions(raw)
	if err != nil {
		return nil, false, adaptor.UnbillableRequestControl(err)
	}
	out := map[string]any{}
	if options.hasMode {
		out["mode"] = options.mode
	}
	if options.hasTTL {
		out["ttl"] = options.ttl
	}
	if options.hasPrewm {
		out["prewarm"] = options.prewarm
	}
	if len(out) == 0 {
		return nil, false, nil
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return nil, false, errors.Wrap(err, "encode OpenAI prompt_cache_options")
	}
	return encoded, true, nil
}

// scanPromptCacheBreakpoints walks provider-owned request structures for
// explicit breakpoints and validates their TTLs. Caller data (tool schemas,
// prompt variables, function arguments, metadata, extension bodies) is never
// interpreted, and strings are never parsed as embedded JSON. An output field
// is walked only as an array of content parts (function_call_output). The walk
// is bounded against deep or wide inputs. Parameters: value is a decoded
// subtree, depth its nesting, nodes the shared work counter and count receives
// the number of breakpoints. It returns a validation or traversal-limit error.
func scanPromptCacheBreakpoints(value any, depth int, nodes, count *int) error {
	if depth > promptCacheScanMaxDepth {
		return errors.New("OpenAI cache breakpoint structure exceeds nesting limit")
	}
	*nodes++
	if *nodes > promptCacheScanMaxNodes {
		return errors.New("OpenAI cache breakpoint structure exceeds traversal limit")
	}
	switch item := value.(type) {
	case []any:
		for _, child := range item {
			if err := scanPromptCacheBreakpoints(child, depth+1, nodes, count); err != nil {
				return err
			}
		}
	case map[string]any:
		if raw, present := item[promptCacheBreakpointKey]; present && raw != nil {
			point, ok := raw.(map[string]any)
			if !ok {
				return errors.New("OpenAI prompt_cache_breakpoint must be an object")
			}
			if rawTTL, present := point["ttl"]; present {
				ttl, ok := rawTTL.(string)
				if !ok {
					return errors.New("OpenAI cache breakpoint TTL must be a string")
				}
				if err := validatePromptCacheTTL(ttl, "OpenAI cache breakpoint TTL"); err != nil {
					return err
				}
			}
			*count++
		}
		for key, child := range item {
			switch key {
			// Tool parameter schemas and prompt-template variables are caller
			// data, not provider cache controls.
			case promptCacheBreakpointKey, "metadata", "arguments", "input_schema", "schema", "parameters", "variables", "data", "extra_body":
				continue
			case "output":
				// function_call_output may carry content parts with breakpoints;
				// any other output shape is provider or caller data.
				if _, parts := child.([]any); !parts {
					continue
				}
			}
			if err := scanPromptCacheBreakpoints(child, depth+1, nodes, count); err != nil {
				return err
			}
		}
	}
	return nil
}

// validatePromptCacheTTL accepts only the default 30m lifetime this gateway
// meters. Parameters: ttl is the requested lifetime and label names the field
// in the error. It returns nil for an empty or 30m TTL (case-insensitive,
// surrounding spaces ignored), else an error naming the unmetered lifetime.
func validatePromptCacheTTL(ttl, label string) error {
	if ttl = strings.TrimSpace(ttl); ttl == "" || strings.EqualFold(ttl, promptCacheTTLDefault) {
		return nil
	}
	return errors.Errorf("%s %q requires a separately metered tariff", label, ttl)
}

// promptCacheString decodes a present JSON string control. Parameters: raw is
// the field value and label names it in errors. It returns the trimmed string
// or an error for null and non-string values, which must not silently become
// a default cache setting.
func promptCacheString(raw json.RawMessage, label string) (string, error) {
	if isJSONNull(raw) {
		return "", errors.Errorf("OpenAI %s must be a string", label)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", errors.Wrapf(err, "decode OpenAI %s as string", label)
	}
	return strings.TrimSpace(value), nil
}

// isJSONNull reports whether raw is absent or the JSON null literal.
// Parameter raw is one decoded field. It returns true for empty or null.
func isJSONNull(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null"))
}
