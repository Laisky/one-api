package main

import (
	"strings"

	"github.com/Laisky/errors/v2"

	cfg "github.com/Laisky/one-api/common/config"
)

// config captures the configuration derived from flags and environment variables.
type config struct {
	APIBase  string
	Token    string
	Models   []string
	Variants []requestVariant
}

// loadConfig constructs the harness configuration from the shared config package.
func loadConfig() (config, error) {
	base := strings.TrimSpace(cfg.APIBase)
	if base == "" {
		base = defaultAPIBase
	}

	token := strings.TrimSpace(cfg.APIToken)
	if token == "" {
		return config{}, errors.Errorf("API_TOKEN must be set")
	}

	modelsRaw := cfg.OneAPITestModels
	models, err := parseModels(modelsRaw)
	if err != nil {
		return config{}, errors.Wrap(err, "parse models")
	}
	if len(models) == 0 {
		models, err = parseModels(defaultTestModels)
		if err != nil {
			return config{}, errors.Wrap(err, "parse default models")
		}
	}

	variantsRaw := cfg.OneAPITestVariants
	variants, err := parseVariants(variantsRaw)
	if err != nil {
		return config{}, errors.Wrap(err, "parse variants")
	}

	return config{
		APIBase:  strings.TrimSuffix(base, "/"),
		Token:    token,
		Models:   models,
		Variants: variants,
	}, nil
}

// parseModels tokenizes ONEAPI_TEST_MODELS into a slice of model identifiers.
func parseModels(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	separators := []string{",", ";", "\n", "\r"}
	normalized := raw
	for _, sep := range separators {
		normalized = strings.ReplaceAll(normalized, sep, ",")
	}

	parts := strings.Split(normalized, ",")
	if len(parts) == 1 && !strings.ContainsAny(raw, ",;\n") {
		parts = strings.Fields(raw)
	}

	var models []string
	for _, part := range parts {
		candidate := strings.TrimSpace(part)
		if candidate == "" {
			continue
		}
		models = append(models, candidate)
	}

	return models, nil
}

// parseVariants resolves ONEAPI_TEST_VARIANTS into the subset of request variants to execute.
func parseVariants(raw string) ([]requestVariant, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return requestVariants, nil
	}

	separators := []string{",", ";", "\n", "\r"}
	normalized := raw
	for _, sep := range separators {
		normalized = strings.ReplaceAll(normalized, sep, ",")
	}

	parts := strings.Split(normalized, ",")
	if len(parts) == 1 && !strings.ContainsAny(raw, ",;\n") {
		parts = strings.Fields(raw)
	}

	selected := make([]requestVariant, 0, len(requestVariants))
	seen := make(map[string]bool, len(requestVariants))
	typeGroups := map[string]requestType{
		"chat":            requestTypeChatCompletion,
		"chat_completion": requestTypeChatCompletion,
		"response":        requestTypeResponseAPI,
		"responses":       requestTypeResponseAPI,
		"response_api":    requestTypeResponseAPI,
		"claude":          requestTypeClaudeMessages,
		"claude_messages": requestTypeClaudeMessages,
	}

	for _, part := range parts {
		candidate := strings.TrimSpace(part)
		if candidate == "" {
			continue
		}

		lower := strings.ToLower(candidate)
		matched := false

		for _, variant := range requestVariants {
			if strings.EqualFold(candidate, variant.Key) || strings.EqualFold(candidate, variant.Header) {
				if !seen[variant.Key] {
					selected = append(selected, variant)
					seen[variant.Key] = true
				}
				matched = true
				break
			}
			for _, alias := range variant.Aliases {
				if strings.EqualFold(candidate, alias) {
					if !seen[variant.Key] {
						selected = append(selected, variant)
						seen[variant.Key] = true
					}
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if matched {
			continue
		}

		if groupType, ok := typeGroups[lower]; ok {
			for _, variant := range requestVariants {
				if variant.Type == groupType && !seen[variant.Key] {
					selected = append(selected, variant)
					seen[variant.Key] = true
				}
			}
			matched = true
		}

		if !matched {
			return nil, errors.Errorf("unknown variant or api format %q", candidate)
		}
	}

	if len(selected) == 0 {
		return nil, errors.New("no variants selected")
	}

	return selected, nil
}
