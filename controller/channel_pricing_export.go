package controller

import (
	"encoding/json"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
)

// exportDefaultModelPricing converts provider defaults through the persisted
// pricing schema. Parameters: defaults contains provider model configurations.
// Returns: independent channel configurations preserving all persistable pricing
// fields, or a serialization error. Provider-only capabilities, provenance, and
// request-specific video quotes are intentionally outside the local schema.
func exportDefaultModelPricing(defaults map[string]adaptor.ModelConfig) (map[string]model.ModelConfigLocal, error) {
	data, err := json.Marshal(defaults)
	if err != nil {
		return nil, errors.Wrap(err, "marshal provider pricing defaults")
	}
	configs := make(map[string]model.ModelConfigLocal)
	if err := json.Unmarshal(data, &configs); err != nil {
		return nil, errors.Wrap(err, "decode channel pricing defaults")
	}
	return configs, nil
}
