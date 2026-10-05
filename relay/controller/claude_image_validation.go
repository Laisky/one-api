package controller

import "github.com/Laisky/errors/v2"

// validateClaudeImageDetails checks image detail fields in content, including tool-result images, and returns an input error for unsupported hints.
func validateClaudeImageDetails(content any) error {
	switch value := content.(type) {
	case []any:
		for _, block := range value {
			if err := validateClaudeImageDetails(block); err != nil {
				return errors.Wrap(err, "validate image block")
			}
		}
	case map[string]any:
		kind, _ := value["type"].(string)
		switch kind {
		case "image":
			source, ok := value["source"].(map[string]any)
			if !ok {
				return nil
			}
			raw, exists := source["detail"]
			if !exists {
				return nil
			}
			detail, ok := raw.(string)
			if !ok {
				return errors.New("image source.detail must be a string")
			}
			switch detail {
			case "", "auto", "low", "high":
				return nil
			default:
				return errors.New("image source.detail must be auto, low, or high")
			}
		case "tool_result", "search_result":
			if err := validateClaudeImageDetails(value["content"]); err != nil {
				return errors.Wrap(err, "validate nested image details")
			}
		}
	}
	return nil
}
