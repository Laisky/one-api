package model

import (
	"encoding/json"

	"github.com/Laisky/errors/v2"
)

// ParseModelMapping decodes a channel's model_mapping column exactly as the
// relay reads a saved channel, so a draft and the persisted row share one
// parser. Parameters: raw is the stored or draft JSON object text and may be
// nil. Returns: nil and no error for a nil, empty, or "{}" value; the decoded
// mapping otherwise; or a wrapped error when raw is not a JSON string map.
func ParseModelMapping(raw *string) (map[string]string, error) {
	if raw == nil || *raw == "" || *raw == "{}" {
		return nil, nil
	}
	modelMapping := make(map[string]string)
	if err := json.Unmarshal([]byte(*raw), &modelMapping); err != nil {
		return nil, errors.Wrap(err, "unmarshal model mapping")
	}
	return modelMapping, nil
}

// ParseModelPriceConfigs decodes a channel's model_configs column exactly as
// the relay reads a saved channel's pricing, so a draft and the persisted row
// share one parser. Parameters: raw is the stored or draft JSON object text
// and may be nil. Returns: nil and no error for a nil, empty, or "{}" value;
// the decoded per-model configurations otherwise; or a wrapped error when raw
// is not a JSON object of model configurations.
func ParseModelPriceConfigs(raw *string) (map[string]ModelConfigLocal, error) {
	if raw == nil || *raw == "" || *raw == "{}" {
		return nil, nil
	}
	modelPriceConfigs := make(map[string]ModelConfigLocal)
	if err := json.Unmarshal([]byte(*raw), &modelPriceConfigs); err != nil {
		return nil, errors.Wrap(err, "unmarshal model price configs")
	}
	return modelPriceConfigs, nil
}
