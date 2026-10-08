package anthropic

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/Laisky/errors/v2"
)

// IsClaudeHaiku55 reports whether name is the published native Haiku 5.5 ID.
// It ignores casing and surrounding whitespace, but never guesses cloud aliases.
func IsClaudeHaiku55(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), "claude-haiku-5-5")
}

// NormalizeHaiku55Controls validates known Haiku 5.5 controls in obj and returns
// an error for incompatible combinations. It changes only top-level controls,
// preserving conversation blocks, signatures, forced tools and unknown fields.
// Sources (verified 2026-10-08):
//   - https://platform.claude.com/docs/en/models/haiku-5-5/migration-guide
//   - https://platform.claude.com/docs/en/build-with-claude/effort
func NormalizeHaiku55Controls(name string, obj map[string]json.RawMessage) error {
	if !IsClaudeHaiku55(name) {
		return nil
	}
	if obj == nil {
		return errors.New("validation failed: Claude request must be an object")
	}
	mode := "adaptive"
	if raw := obj["thinking"]; len(raw) > 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		var thinking map[string]json.RawMessage
		if err := json.Unmarshal(raw, &thinking); err != nil {
			return errors.Wrap(err, "validation failed: thinking must be an object")
		}
		mode = ""
		if err := json.Unmarshal(thinking["type"], &mode); err != nil {
			return errors.Wrap(err, "validation failed: thinking.type must be a string")
		}
		switch mode {
		case "enabled":
			mode = "adaptive"
			thinking["type"] = json.RawMessage(`"adaptive"`)
		case "adaptive", "disabled":
		default:
			return errors.New("validation failed: unsupported Haiku 5.5 thinking.type")
		}
		delete(thinking, "budget_tokens")
		normalized, err := json.Marshal(thinking)
		if err != nil {
			return errors.Wrap(err, "validation failed: encode Haiku thinking")
		}
		obj["thinking"] = normalized
	}

	effort := "medium"
	if raw := obj["output_config"]; len(raw) > 0 {
		var output map[string]json.RawMessage
		if err := json.Unmarshal(raw, &output); err != nil {
			return errors.Wrap(err, "validation failed: output_config must be an object")
		}
		if output == nil {
			return errors.New("validation failed: output_config must be an object")
		}
		if rawEffort, exists := output["effort"]; exists {
			if bytes.Equal(bytes.TrimSpace(rawEffort), []byte("null")) {
				return errors.New("validation failed: output_config.effort must be a string")
			}
			if err := json.Unmarshal(rawEffort, &effort); err != nil {
				return errors.Wrap(err, "validation failed: output_config.effort must be a string")
			}
		}
	}
	switch effort {
	case "low", "medium", "high":
	case "xhigh", "max":
		if mode == "disabled" {
			return errors.New("validation failed: disabled Haiku thinking requires low, medium or high effort")
		}
	default:
		return errors.New("validation failed: unsupported Haiku 5.5 effort")
	}

	if raw := obj["messages"]; len(raw) > 0 {
		var messages []struct {
			Role         string                     `json:"role"`
			OutputConfig map[string]json.RawMessage `json:"output_config"`
		}
		if err := json.Unmarshal(raw, &messages); err != nil {
			return errors.Wrap(err, "validation failed: invalid Claude messages")
		}
		if len(messages) > 0 && messages[len(messages)-1].Role == "assistant" {
			return errors.New("validation failed: Haiku 5.5 does not support assistant prefill")
		}
		if mode == "disabled" {
			for _, message := range messages {
				if rawEffort, exists := message.OutputConfig["effort"]; exists {
					var previous string
					if err := json.Unmarshal(rawEffort, &previous); err != nil {
						return errors.Wrap(err, "validation failed: invalid per-message effort")
					}
					if previous != effort {
						return errors.New("validation failed: disabled Haiku thinking cannot change effort mid-conversation")
					}
				}
			}
		}
	}
	for _, key := range []string{"temperature", "top_p", "top_k"} {
		delete(obj, key)
	}
	return nil
}
