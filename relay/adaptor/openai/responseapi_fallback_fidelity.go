package openai

import (
	"strings"

	"github.com/Laisky/errors/v2"
)

// ErrResponseNamespaceUnsupported identifies call identities that require native Responses support.
var ErrResponseNamespaceUnsupported = errors.New("tool namespaces require native Responses support")

// ErrResponseFallbackNotRepresentable identifies input with no faithful stateless representation.
var ErrResponseFallbackNotRepresentable = errors.New("response input cannot be represented on this fallback route")

// ErrResponseFallbackStateNotPortable identifies canonical state with no sanctioned fallback lowering.
// It retains the generic fidelity identity for callers that only need admission failure.
var ErrResponseFallbackStateNotPortable = errors.Wrap(ErrResponseFallbackNotRepresentable, "state item is not portable")

// ValidateResponseAPIFallbackFidelity rejects semantic loss before fallback work.
// allowReferences permits owner-scoped resolution before hydration only. Reasoning
// and thinking retain their existing sidecar degradation policy; this validation
// does not promote opaque state into text or inspect function arguments/results.
func ValidateResponseAPIFallbackFidelity(input []any, allowReferences bool) error {
	for _, raw := range input {
		item, ok := raw.(map[string]any)
		if !ok {
			if _, ok := raw.(string); ok {
				continue
			}
			return errors.Wrap(ErrResponseFallbackNotRepresentable, "unsupported input item")
		}
		kind, _ := item["type"].(string)
		switch strings.ToLower(kind) {
		case "function_call", "function_call_output", "custom_tool_call", "custom_tool_call_output":
			if namespace, exists := item["namespace"]; exists && namespace != nil && namespace != "" {
				return errors.WithStack(ErrResponseNamespaceUnsupported)
			}
			if strings.HasPrefix(strings.ToLower(kind), "custom_") {
				return errors.Wrap(ErrResponseFallbackStateNotPortable, "custom tool history requires native Responses support")
			}
		case "reasoning", "thinking":
			// These provider-bound sidecars intentionally degrade on fallback.
		case "item_reference":
			if !allowReferences {
				return errors.Wrap(ErrResponseFallbackNotRepresentable, "unresolved item reference")
			}
		case "", "message":
			if err := validateResponseFallbackContent(item["content"]); err != nil {
				return err
			}
		default:
			return errors.Wrap(ErrResponseFallbackStateNotPortable, "unsupported response item kind")
		}
	}
	return nil
}

// validateResponseFallbackContent accepts only content the converter represents.
func validateResponseFallbackContent(raw any) error {
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
			case "input_file":
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
