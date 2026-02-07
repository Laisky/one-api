package pricing

import (
	"testing"

	"github.com/songquanpeng/one-api/relay/adaptor"
)

// setTestGlobalModelConfigs replaces global model pricing for deterministic tests.
// Parameters: t is the current test handle; configs defines the global pricing table.
// Returns: nothing. The original global pricing manager is restored via test cleanup.
func setTestGlobalModelConfigs(t *testing.T, configs map[string]adaptor.ModelConfig) {
	t.Helper()

	previous := globalPricingManager
	cloned := make(map[string]adaptor.ModelConfig, len(configs))
	for modelName, cfg := range configs {
		cloned[modelName] = cloneModelConfig(cfg)
	}

	globalPricingManager = &GlobalPricingManager{
		globalModelPricing: cloned,
		initialized:        true,
	}

	t.Cleanup(func() {
		globalPricingManager = previous
	})
}
