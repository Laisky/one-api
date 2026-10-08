package controller

import (
	"encoding/json"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/gin-gonic/gin"
)

const (
	tierAdmissionProviderBodyKey = "tier_admission_provider_body"
	tierAdmissionCacheMaxDepth   = 64
	tierAdmissionCacheMaxNodes   = 100000
)

// tierAdmissionCacheEligibility tracks eligible cache-write buckets and bounds
// structural traversal of a parsed provider request.
type tierAdmissionCacheEligibility struct {
	write5m bool
	write1h bool
	nodes   int
}

// tierAdmissionCacheWrites inspects a parsed request's root cache control,
// system, messages, and tools for structural ephemeral cache controls. Parameters: request is the
// canonical parsed payload; strings are never parsed as embedded JSON. Returns:
// five-minute and one-hour write eligibility, or an error when serialization,
// cache controls, or bounded structural traversal cannot be validated.
func tierAdmissionCacheWrites(request any) (bool, bool, error) {
	encoded, err := json.Marshal(request)
	if err != nil {
		return false, false, errors.Wrap(err, "encode tier admission cache request")
	}
	var root map[string]any
	if err := json.Unmarshal(encoded, &root); err != nil {
		return false, false, errors.Wrap(err, "decode tier admission cache request")
	}
	if root == nil {
		return false, false, errors.WithStack(errors.New("tier admission cache request must be an object"))
	}

	var eligibility tierAdmissionCacheEligibility
	if control, exists := root["cache_control"]; exists && control != nil {
		if err := tierAdmissionCacheControl(control, &eligibility); err != nil {
			return false, false, errors.Wrap(err, "inspect tier admission root cache control")
		}
	}
	for _, key := range []string{"system", "messages", "tools"} {
		if err := tierAdmissionCacheWalk(root[key], 0, &eligibility); err != nil {
			return false, false, errors.Wrap(err, "inspect tier admission cache controls")
		}
	}
	return eligibility.write5m, eligibility.write1h, nil
}

// tierAdmissionCacheWalk visits structural request values without interpreting
// user strings, metadata, transport extras, schemas, or tool input as cache
// controls. Parameters: value is a parsed subtree, depth is its nesting depth,
// and eligibility receives eligible write buckets and the traversal count.
// Returns: an error for excessive nesting, work, or malformed cache controls.
func tierAdmissionCacheWalk(value any, depth int, eligibility *tierAdmissionCacheEligibility) error {
	if depth > tierAdmissionCacheMaxDepth {
		return errors.WithStack(errors.New("tier admission cache structure exceeds nesting limit"))
	}
	eligibility.nodes++
	if eligibility.nodes > tierAdmissionCacheMaxNodes {
		return errors.WithStack(errors.New("tier admission cache structure exceeds traversal limit"))
	}

	switch node := value.(type) {
	case []any:
		for _, child := range node {
			if err := tierAdmissionCacheWalk(child, depth+1, eligibility); err != nil {
				return err
			}
		}
	case map[string]any:
		if control, exists := node["cache_control"]; exists && control != nil {
			if err := tierAdmissionCacheControl(control, eligibility); err != nil {
				return err
			}
		}
		for key, child := range node {
			switch key {
			case "cache_control", "metadata", "extra_body", "input_schema", "schema", "input", "arguments", "data":
				continue
			}
			if err := tierAdmissionCacheWalk(child, depth+1, eligibility); err != nil {
				return err
			}
		}
	}
	return nil
}

// tierAdmissionCacheControl records a structural ephemeral cache control.
// Parameters: value is the parsed cache_control object and eligibility receives
// the matching write bucket. Returns: an error for malformed control structure
// or an unsupported ephemeral TTL; other control types do not imply eligibility.
func tierAdmissionCacheControl(value any, eligibility *tierAdmissionCacheEligibility) error {
	control, ok := value.(map[string]any)
	if !ok {
		return errors.WithStack(errors.New("tier admission cache_control must be an object"))
	}
	if control["type"] != "ephemeral" {
		return nil
	}
	ttl, exists := control["ttl"]
	if !exists || ttl == nil {
		eligibility.write5m = true
		return nil
	}
	switch ttl {
	case "", "5m":
		eligibility.write5m = true
	case "1h":
		eligibility.write1h = true
	default:
		return errors.WithStack(errors.New("tier admission ephemeral cache_control has unsupported ttl"))
	}
	return nil
}

// tierAdmissionPreparedPayload selects the exact serialized provider payload
// captured during preparation, then the converted request, then the supplied
// fallback. Parameters: c contains preparation state and fallback is the parsed
// canonical request. Returns: a raw JSON payload or parsed request without
// reading, consuming, or changing the upstream request body.
func tierAdmissionPreparedPayload(c *gin.Context, fallback any) any {
	if c == nil {
		return fallback
	}
	if stored, exists := c.Get(tierAdmissionProviderBodyKey); exists {
		if body, ok := stored.([]byte); ok && len(body) > 0 {
			return json.RawMessage(body)
		}
	}
	if converted, exists := c.Get(ctxkey.ConvertedRequest); exists && converted != nil {
		return converted
	}
	return fallback
}
