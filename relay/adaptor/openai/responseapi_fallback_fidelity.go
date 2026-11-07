package openai

import (
	"encoding/json"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/one-api/relay/model"
)

// ErrResponseFallbackNotRepresentable identifies input with no faithful stateless representation.
var ErrResponseFallbackNotRepresentable = errors.New("response input cannot be represented on this fallback route")

// ErrResponseFallbackStateNotPortable identifies canonical state with no sanctioned fallback lowering.
// It retains the generic fidelity identity for callers that only need admission failure.
var ErrResponseFallbackStateNotPortable = errors.Wrap(ErrResponseFallbackNotRepresentable, "state item is not portable")

// ValidateResponseAPIFallbackFidelity rejects semantic loss before fallback work.
// allowReferences permits owner-scoped resolution before hydration only. Reasoning
// and thinking retain their existing sidecar degradation policy; this validation
// does not promote opaque state into text or inspect function arguments. Function
// and custom tool results are inspected only when they carry input_image parts:
// those are validated like user-message content and capped at
// maxResponseFallbackToolOutputImages, while every other result shape keeps its
// historical JSON stringification. agent_message items are admitted only when
// lowerResponseAgentMessage can lower them: string content or an array of text
// parts. It returns nil, or an error wrapping ErrResponseFallbackNotRepresentable
// (ErrResponseFallbackStateNotPortable for unknown item kinds) that names the
// rejected shape without quoting input content.
func ValidateResponseAPIFallbackFidelity(input []any, allowReferences bool, nativeMedia ...bool) error {
	toolOutputImages := 0
	for _, raw := range input {
		item, ok := raw.(map[string]any)
		if !ok {
			if _, ok := raw.(string); ok {
				continue
			}
			if responseFallbackDiscoveryItem(raw) {
				// Discovery items stay raw to keep exact literals; their shape is
				// validated by discovery admission and lowered by the catalog.
				continue
			}
			return errors.Wrap(ErrResponseFallbackNotRepresentable, "unsupported input item")
		}
		kind, _ := item["type"].(string)
		switch strings.ToLower(kind) {
		case "function_call", "function_call_output", "custom_tool_call", "custom_tool_call_output":
			// Namespaced and custom calls are flattened through the fallback tool
			// catalog, which keeps the qualified identity reversible.
			if namespace, exists := item["namespace"]; exists && namespace != nil {
				if _, ok := namespace.(string); !ok {
					return errors.Wrap(ErrResponseFallbackNotRepresentable, "tool call namespace must be a string")
				}
			}
			if strings.HasSuffix(strings.ToLower(kind), "_output") {
				// Image-bearing outputs (Codex view_image and similar) are lowered
				// as images, so they are validated like user-message content and
				// bounded; every other output shape keeps its stringification.
				images, err := validateResponseFallbackToolOutput(responseToolOutputValue(item))
				if err != nil {
					return err
				}
				toolOutputImages += images
				if toolOutputImages > maxResponseFallbackToolOutputImages {
					return errors.Wrapf(ErrResponseFallbackNotRepresentable,
						"tool outputs carry more than %d images", maxResponseFallbackToolOutputImages)
				}
			}
		case "tool_search_call", "tool_search_output", "additional_tools", "configuration_update":
			// Client discovery and harness controls are validated by the
			// discovery admission and lowered by lowerResponseFallbackToolItems.
		case "agent_message":
			// Admission runs the same lowering the converter uses, so content
			// that would lose the agent's text is rejected before dispatch.
			if _, err := lowerResponseAgentMessage(item); err != nil {
				return err
			}
		case "reasoning", "thinking":
			// These provider-bound sidecars intentionally degrade on fallback.
		case "web_search_call":
			// Hosted search history (for example Codex replaying its own turns)
			// is dropped: the assistant text already carries what the search
			// found, and no stateless upstream can resume provider search state.
		case "item_reference":
			if !allowReferences {
				return errors.Wrap(ErrResponseFallbackNotRepresentable, "unresolved item reference")
			}
		case "", "message":
			if err := validateResponseFallbackContent(item["content"], nativeMedia...); err != nil {
				return err
			}
		default:
			return errors.Wrap(ErrResponseFallbackStateNotPortable, "unsupported response item kind")
		}
	}
	return nil
}

// validateResponseFallbackContent accepts only content the converter represents.
func validateResponseFallbackContent(raw any, nativeMedia ...bool) error {
	switch content := raw.(type) {
	case nil, string:
		return nil
	case []any:
		for _, rawPart := range content {
			part, ok := rawPart.(map[string]any)
			if !ok {
				return errors.Wrap(ErrResponseFallbackNotRepresentable, "unsupported content part")
			}
			kind, _ := part["type"].(string)
			switch kind {
			case "", "text", "input_text", "output_text":
				// Missing text retains the historical empty assistant/tool-call form.
				if text, present := part["text"]; present {
					if _, ok := text.(string); !ok {
						return errors.Wrap(ErrResponseFallbackNotRepresentable, "invalid text part")
					}
				}
			case "reasoning":
				// Existing plaintext reasoning sidecars remain supported.
			case "refusal":
				if _, ok := part["refusal"].(string); !ok {
					return errors.Wrap(ErrResponseFallbackNotRepresentable, "invalid refusal part")
				}
			case "input_image":
				if url, _ := part["image_url"].(string); url != "" {
					continue
				}
				if !responseFallbackFileRepresentable(part) {
					return errors.Wrap(ErrResponseFallbackNotRepresentable, "unsupported image part")
				}
			case "input_file", "input_video", "video_url":
				if len(nativeMedia) > 0 && nativeMedia[0] {
					if _, err := (model.Message{Content: []any{part}}).ParseContentChecked(); err != nil {
						return errors.Wrap(ErrResponseFallbackNotRepresentable, "invalid native media part")
					}
					continue
				}
				// The shared Chat content model uses a legacy flat file shape,
				// not the nested Chat file protocol. Do not dispatch invalid
				// wire data or infer a provider-uploaded file's type here.
				return errors.Wrap(ErrResponseFallbackNotRepresentable, "file inputs require native Responses support")
			case "input_audio":
				audio, ok := part["input_audio"].(map[string]any)
				if !ok {
					return errors.Wrap(ErrResponseFallbackNotRepresentable, "unsupported audio part")
				}
				data, _ := audio["data"].(string)
				format, _ := audio["format"].(string)
				if data == "" || format == "" {
					return errors.Wrap(ErrResponseFallbackNotRepresentable, "invalid audio part")
				}
			default:
				return errors.Wrap(ErrResponseFallbackNotRepresentable, "unsupported content part kind")
			}
		}
		return nil
	default:
		return errors.Wrap(ErrResponseFallbackNotRepresentable, "unsupported message content")
	}
}

// responseFallbackFileRepresentable reports whether the existing Chat file fields retain the attachment.
func responseFallbackFileRepresentable(part map[string]any) bool {
	fileID, _ := part["file_id"].(string)
	fileData, _ := part["file_data"].(string)
	if fileURL, exists := part["file_url"]; exists && fileURL != nil && fileURL != "" {
		return false
	}
	return fileID != "" || fileData != ""
}

// cloneResponseAPIJSONValue detaches JSON-decoded schema maps and arrays before channel normalization.
func cloneResponseAPIJSONValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		cloned := make(map[string]any, len(value))
		for key, item := range value {
			cloned[key] = cloneResponseAPIJSONValue(item)
		}
		return cloned
	case []any:
		cloned := make([]any, len(value))
		for i, item := range value {
			cloned[i] = cloneResponseAPIJSONValue(item)
		}
		return cloned
	case []string:
		return append([]string(nil), value...)
	default:
		return value
	}
}

// responseFallbackDiscoveryItem reports whether raw is a raw-preserved client
// discovery item (tool_search_call, tool_search_output or additional_tools).
func responseFallbackDiscoveryItem(raw any) bool {
	data, ok := raw.(json.RawMessage)
	if !ok {
		return false
	}
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(probe.Type)) {
	case "tool_search_call", "tool_search_output", "additional_tools":
		return true
	default:
		return false
	}
}
