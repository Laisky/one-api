package anthropic

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/gin-gonic/gin"
)

// IsClaudeSonnet55 matches the published native ID, not third-party model namespaces.
func IsClaudeSonnet55(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), "claude-sonnet-5-5")
}

// CompatibilityModel resolves a known Azure origin for an opaque deployment name.
// A mapped canonical model takes precedence; other providers never inherit an origin policy.
func CompatibilityModel(c *gin.Context, wireModel string) string {
	if _, known := ModelRatios[wireModel]; known || c == nil {
		return wireModel
	}
	value, exists := c.Get(ctxkey.Meta)
	if !exists {
		return wireModel
	}
	m, ok := value.(*meta.Meta)
	if !ok || m == nil || m.ChannelType != channeltype.Azure {
		return wireModel
	}
	if _, known := ModelRatios[m.OriginModelName]; known {
		return m.OriginModelName
	}
	return wireModel
}

// NormalizeSonnet55Controls validates and normalizes only the documented Sonnet 5.5 controls.
// It mutates top-level raw fields, never conversation content or unrelated extensions.
// Source: https://platform.claude.com/docs/en/models/sonnet-5-5/migration-guide (2026-09-28).
func NormalizeSonnet55Controls(name string, obj map[string]json.RawMessage) error {
	if !IsClaudeSonnet55(name) {
		return nil
	}
	if obj == nil {
		return errors.New("validation failed: Claude request must be an object")
	}
	var thinking map[string]json.RawMessage
	mode := "adaptive"
	if raw := obj["thinking"]; len(raw) > 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if err := json.Unmarshal(raw, &thinking); err != nil {
			return errors.Wrap(err, "validation failed: thinking must be an object")
		}
		if err := json.Unmarshal(thinking["type"], &mode); err != nil {
			return errors.Wrap(err, "validation failed: thinking.type must be a string")
		}
		switch mode {
		case "disabled":
			mode = "between_tools"
			thinking["type"] = json.RawMessage(`"between_tools"`)
		case "enabled":
			mode = "adaptive"
			thinking["type"] = json.RawMessage(`"adaptive"`)
			delete(thinking, "budget_tokens")
		case "adaptive", "between_tools":
		default:
			return errors.New("validation failed: unsupported Sonnet 5.5 thinking.type")
		}
		if mode == "between_tools" && len(thinking) != 1 {
			return errors.New("validation failed: between_tools accepts only type, without display, budget_tokens or block_binding")
		}
		if mode == "adaptive" {
			delete(thinking, "budget_tokens")
		}
		raw, err := json.Marshal(thinking)
		if err != nil {
			return errors.Wrap(err, "validation failed: encode thinking")
		}
		obj["thinking"] = raw
	}
	effort := "high"
	if raw := obj["output_config"]; len(raw) > 0 {
		var output map[string]json.RawMessage
		if err := json.Unmarshal(raw, &output); err != nil {
			return errors.Wrap(err, "validation failed: output_config must be an object")
		}
		if output == nil {
			return errors.New("validation failed: output_config must be an object")
		}
		if rawEffort, exists := output["effort"]; exists {
			if err := json.Unmarshal(rawEffort, &effort); err != nil {
				return errors.Wrap(err, "validation failed: output_config.effort must be a string")
			}
		}
	}
	switch effort {
	case "low", "medium", "high":
	case "xhigh", "max":
		if mode == "between_tools" {
			return errors.New("validation failed: between_tools requires low, medium or high effort")
		}
	default:
		return errors.New("validation failed: unsupported Sonnet 5.5 effort")
	}
	if raw := obj["tool_choice"]; len(raw) > 0 && string(raw) != "null" {
		var kind string
		if err := json.Unmarshal(raw, &kind); err != nil {
			var choice struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(raw, &choice); err != nil {
				return errors.Wrap(err, "validation failed: invalid tool_choice")
			}
			kind = choice.Type
		}
		switch kind {
		case "auto", "none":
		default:
			return errors.New("validation failed: Sonnet 5.5 does not support forced tool_choice; choose auto or none explicitly")
		}
	}
	if len(obj["messages"]) > 0 {
		var messages []struct {
			Role         string                     `json:"role"`
			OutputConfig map[string]json.RawMessage `json:"output_config"`
		}
		if err := json.Unmarshal(obj["messages"], &messages); err != nil {
			return errors.Wrap(err, "validation failed: invalid Claude messages")
		}
		if len(messages) > 0 && messages[len(messages)-1].Role == "assistant" {
			return errors.New("validation failed: Sonnet 5.5 does not support assistant prefill")
		}
		for _, message := range messages {
			if mode != "between_tools" {
				break
			}
			if raw, exists := message.OutputConfig["effort"]; exists {
				var perMessage string
				if err := json.Unmarshal(raw, &perMessage); err != nil {
					return errors.Wrap(err, "validation failed: invalid per-message effort")
				}
				if perMessage != effort {
					return errors.New("validation failed: between_tools cannot change effort mid-conversation")
				}
			}
		}
	}
	for _, key := range []string{"temperature", "top_p", "top_k"} {
		delete(obj, key)
	}
	return nil
}
